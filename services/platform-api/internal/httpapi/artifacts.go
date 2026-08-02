package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	artifactapp "github.com/sid995/agentforge/services/platform-api/internal/application/artifacts"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/identity"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

const defaultArtifactDownloadLifetime = 5 * time.Minute

type artifactResponse struct {
	ID             string `json:"id"`
	AttemptNumber  int    `json:"attemptNumber"`
	ContentType    string `json:"contentType"`
	SizeBytes      int64  `json:"sizeBytes"`
	SHA256         string `json:"sha256"`
	RetentionClass string `json:"retentionClass"`
	CreatedAt      string `json:"createdAt"`
}

type artifactsResponse struct {
	Data []artifactResponse `json:"data"`
}

type artifactDownloadResponse struct {
	Data struct {
		URL       string `json:"url"`
		ExpiresAt string `json:"expiresAt"`
	} `json:"data"`
}

type deleteArtifactRequest struct {
	Reason string `json:"reason"`
}

func (api *API) listArtifacts(writer http.ResponseWriter, request *http.Request) {
	caller, ok := api.authenticate(writer, request)
	if !ok {
		return
	}
	runID, ok := pathUUID(writer, request, "runId")
	if !ok {
		return
	}
	artifacts, err := api.artifacts.List(request.Context(), caller, runID)
	if err != nil {
		writeArtifactError(writer, request, err)
		return
	}
	response := artifactsResponse{Data: make([]artifactResponse, 0, len(artifacts))}
	for _, artifact := range artifacts {
		response.Data = append(response.Data, responseForArtifact(artifact))
	}
	writer.Header().Set("Content-Type", jsonContentType)
	_ = json.NewEncoder(writer).Encode(response)
}

func (api *API) getArtifact(writer http.ResponseWriter, request *http.Request) {
	caller, ok := api.authenticate(writer, request)
	if !ok {
		return
	}
	runID, ok := pathUUID(writer, request, "runId")
	if !ok {
		return
	}
	artifactID, ok := pathUUID(writer, request, "artifactId")
	if !ok {
		return
	}
	artifact, err := api.artifacts.Get(request.Context(), caller, runID, artifactID)
	if err != nil {
		writeArtifactError(writer, request, err)
		return
	}
	writer.Header().Set("Content-Type", jsonContentType)
	_ = json.NewEncoder(writer).Encode(struct {
		Data artifactResponse `json:"data"`
	}{Data: responseForArtifact(artifact)})
}

func (api *API) downloadArtifact(writer http.ResponseWriter, request *http.Request) {
	caller, ok := api.authenticate(writer, request)
	if !ok {
		return
	}
	runID, ok := pathUUID(writer, request, "runId")
	if !ok {
		return
	}
	artifactID, ok := pathUUID(writer, request, "artifactId")
	if !ok {
		return
	}
	lifetime, err := parseArtifactDownloadLifetime(request)
	if err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "VALIDATION", "Download lifetime is invalid.", false)
		return
	}
	download, err := api.artifacts.Download(request.Context(), caller, runID, artifactID, lifetime)
	if err != nil {
		writeArtifactError(writer, request, err)
		return
	}
	if download.URL == nil || download.ExpiresAt.IsZero() {
		writeError(writer, request, http.StatusInternalServerError, "INTERNAL", "An internal error occurred.", false)
		return
	}
	response := artifactDownloadResponse{}
	response.Data.URL = download.URL.String()
	response.Data.ExpiresAt = download.ExpiresAt.UTC().Format(time.RFC3339Nano)
	writer.Header().Set("Content-Type", jsonContentType)
	_ = json.NewEncoder(writer).Encode(response)
}

func (api *API) deleteArtifact(writer http.ResponseWriter, request *http.Request) {
	caller, ok := api.authenticate(writer, request)
	if !ok {
		return
	}
	runID, ok := pathUUID(writer, request, "runId")
	if !ok {
		return
	}
	artifactID, ok := pathUUID(writer, request, "artifactId")
	if !ok {
		return
	}
	var body deleteArtifactRequest
	if err := decodeJSON(request, &body); err != nil {
		writeRequestDecodeError(writer, request, err)
		return
	}
	if err := api.artifacts.Delete(request.Context(), caller, runID, artifactID, body.Reason); err != nil {
		writeArtifactError(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func responseForArtifact(artifact domain.Artifact) artifactResponse {
	return artifactResponse{ID: artifact.ID.String(), AttemptNumber: artifact.AttemptNumber, ContentType: artifact.ContentType, SizeBytes: artifact.SizeBytes, SHA256: artifact.SHA256, RetentionClass: string(artifact.RetentionClass), CreatedAt: artifact.CreatedAt.UTC().Format(time.RFC3339Nano)}
}

func parseArtifactDownloadLifetime(request *http.Request) (time.Duration, error) {
	seconds := int(defaultArtifactDownloadLifetime / time.Second)
	if value := request.URL.Query().Get("expiresInSeconds"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, err
		}
		seconds = parsed
	}
	lifetime := time.Duration(seconds) * time.Second
	if lifetime <= 0 || lifetime > ports.MaxArtifactPresignLifetime {
		return 0, errors.New("artifact download lifetime is outside the approved range")
	}
	return lifetime, nil
}

func writeArtifactError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "NOT_FOUND", "The requested resource was not found.", false)
	case errors.Is(err, identity.ErrUnauthorized), errors.Is(err, domain.ErrForbidden):
		writeError(writer, request, http.StatusForbidden, "AUTHORIZATION", "The caller is not authorized for this resource.", false)
	case errors.Is(err, domain.ErrValidation):
		writeError(writer, request, http.StatusUnprocessableEntity, "VALIDATION", "Request fields are invalid.", false)
	case errors.Is(err, artifactapp.ErrDownloadUnavailable), errors.Is(err, artifactapp.ErrDeletionUnavailable):
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Artifact storage is not currently available.", true)
	default:
		writeError(writer, request, http.StatusInternalServerError, "INTERNAL", "An internal error occurred.", false)
	}
}
