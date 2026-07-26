package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/application/runs"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type createRunRequest struct {
	PromptReference string `json:"promptRef"`
	Runtime         string `json:"runtime"`
	Resources       struct {
		CPUMillis int `json:"cpuMillis"`
		MemoryMiB int `json:"memoryMiB"`
	} `json:"resources"`
	TimeoutSeconds int `json:"timeoutSeconds"`
	MaxAttempts    int `json:"maxAttempts"`
}

type runResponse struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Runtime   string `json:"runtime"`
	Resources struct {
		CPUMillis int `json:"cpuMillis"`
		MemoryMiB int `json:"memoryMiB"`
	} `json:"resources"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	MaxAttempts    int    `json:"maxAttempts"`
	AttemptCount   int    `json:"attemptCount"`
	Status         string `json:"status"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type runsResponse struct {
	Data       []runResponse `json:"data"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

type runCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

func (api *API) createRun(writer http.ResponseWriter, request *http.Request) {
	caller, ok := api.authenticate(writer, request)
	if !ok {
		return
	}
	projectID, ok := pathUUID(writer, request, "projectId")
	if !ok {
		return
	}
	var body createRunRequest
	if err := decodeJSON(request, &body); err != nil {
		writeRequestDecodeError(writer, request, err)
		return
	}
	run, replayed, err := api.runs.Create(request.Context(), caller, projectID, runs.CreateInput{
		PromptReference: body.PromptReference, Runtime: body.Runtime, CPUMillis: body.Resources.CPUMillis,
		MemoryMiB: body.Resources.MemoryMiB, TimeoutSeconds: body.TimeoutSeconds, MaxAttempts: body.MaxAttempts,
		IdempotencyKey: request.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		writeRunError(writer, request, err)
		return
	}
	if replayed {
		writer.Header().Set("Idempotent-Replay", "true")
	}
	writer.Header().Set("Content-Type", jsonContentType)
	writer.Header().Set("Location", "/v1/runs/"+run.ID.String())
	writer.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(writer).Encode(struct {
		Data runResponse `json:"data"`
	}{Data: responseForRun(run)})
}

func (api *API) getRun(writer http.ResponseWriter, request *http.Request) {
	caller, ok := api.authenticate(writer, request)
	if !ok {
		return
	}
	runID, ok := pathUUID(writer, request, "runId")
	if !ok {
		return
	}
	run, err := api.runs.Get(request.Context(), caller, runID)
	if err != nil {
		writeRunError(writer, request, err)
		return
	}
	writer.Header().Set("Content-Type", jsonContentType)
	_ = json.NewEncoder(writer).Encode(struct {
		Data runResponse `json:"data"`
	}{Data: responseForRun(run)})
}

func (api *API) listRuns(writer http.ResponseWriter, request *http.Request) {
	caller, ok := api.authenticate(writer, request)
	if !ok {
		return
	}
	projectID, ok := pathUUID(writer, request, "projectId")
	if !ok {
		return
	}
	page, err := parseRunPage(request)
	if err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "VALIDATION", "Pagination parameters are invalid.", false)
		return
	}
	items, err := api.runs.List(request.Context(), caller, projectID, page)
	if err != nil {
		writeRunError(writer, request, err)
		return
	}
	response := runsResponse{Data: make([]runResponse, 0, len(items))}
	for _, item := range items {
		response.Data = append(response.Data, responseForRun(item))
	}
	if len(items) == page.Limit && len(items) > 0 {
		response.NextCursor, err = encodeCursor(items[len(items)-1])
		if err != nil {
			writeError(writer, request, http.StatusInternalServerError, "INTERNAL", "An internal error occurred.", false)
			return
		}
	}
	writer.Header().Set("Content-Type", jsonContentType)
	_ = json.NewEncoder(writer).Encode(response)
}

func responseForRun(run domain.AgentRun) runResponse {
	response := runResponse{ID: run.ID.String(), ProjectID: run.ProjectID.String(), Runtime: run.Runtime, TimeoutSeconds: run.TimeoutSeconds, MaxAttempts: run.MaxAttempts, AttemptCount: run.AttemptCount, Status: string(run.Status), CreatedAt: run.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: run.UpdatedAt.UTC().Format(time.RFC3339Nano)}
	response.Resources.CPUMillis = run.CPUMillis
	response.Resources.MemoryMiB = run.MemoryMiB
	return response
}

func pathUUID(writer http.ResponseWriter, request *http.Request, name string) (uuid.UUID, bool) {
	value, err := uuid.Parse(request.PathValue(name))
	if err != nil || value == uuid.Nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "VALIDATION", "Path identifier must be a UUID.", false)
		return uuid.Nil, false
	}
	return value, true
}

func decodeJSON(request *http.Request, destination any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("request must contain a single JSON value")
		}
		return err
	}
	return nil
}

func writeRequestDecodeError(writer http.ResponseWriter, request *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(writer, request, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Request body exceeds the allowed size.", false)
		return
	}
	writeError(writer, request, http.StatusUnprocessableEntity, "VALIDATION", "Request body is invalid.", false)
}

func writeRunError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "NOT_FOUND", "The requested resource was not found.", false)
	case errors.Is(err, runs.ErrIdempotencyKeyReused):
		writeError(writer, request, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "The idempotency key was used with a different request.", false)
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrVersionConflict):
		writeError(writer, request, http.StatusConflict, "CONFLICT", "The request conflicts with current state.", false)
	case errors.Is(err, domain.ErrForbidden):
		writeError(writer, request, http.StatusForbidden, "AUTHORIZATION", "The caller is not authorized for this resource.", false)
	default:
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must ") || strings.Contains(err.Error(), "outside supported bounds") {
			writeError(writer, request, http.StatusUnprocessableEntity, "VALIDATION", "Request fields are invalid.", false)
			return
		}
		writeError(writer, request, http.StatusInternalServerError, "INTERNAL", "An internal error occurred.", false)
	}
}

func parseRunPage(request *http.Request) (ports.AgentRunPage, error) {
	page := ports.AgentRunPage{Limit: 50}
	if limit := request.URL.Query().Get("limit"); limit != "" {
		parsed, err := strconv.Atoi(limit)
		if err != nil || parsed < 1 || parsed > 100 {
			return ports.AgentRunPage{}, fmt.Errorf("invalid limit")
		}
		page.Limit = parsed
	}
	if encoded := request.URL.Query().Get("cursor"); encoded != "" {
		cursor, err := decodeCursor(encoded)
		if err != nil {
			return ports.AgentRunPage{}, err
		}
		id, err := uuid.Parse(cursor.ID)
		if err != nil || id == uuid.Nil || cursor.CreatedAt.IsZero() {
			return ports.AgentRunPage{}, fmt.Errorf("invalid cursor")
		}
		createdAt := cursor.CreatedAt.UTC()
		page.AfterCreatedAt = &createdAt
		page.AfterID = &id
	}
	return page, nil
}

func encodeCursor(run domain.AgentRun) (string, error) {
	payload, err := json.Marshal(runCursor{CreatedAt: run.CreatedAt.UTC(), ID: run.ID.String()})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(encoded string) (runCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return runCursor{}, err
	}
	var cursor runCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return runCursor{}, err
	}
	return cursor, nil
}
