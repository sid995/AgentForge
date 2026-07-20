package httpapi

import (
	"encoding/json"
	"net/http"
)

type errorBody struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
	Retryable bool   `json:"retryable"`
}

func writeError(writer http.ResponseWriter, request *http.Request, status int, code, message string, retryable bool) {
	writer.Header().Set("Content-Type", jsonContentType)
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(errorBody{Error: apiError{
		Code:      code,
		Message:   message,
		RequestID: requestIDFromContext(request.Context()),
		Retryable: retryable,
	}})
}
