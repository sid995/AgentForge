package httpapi

import (
	"encoding/json"
	"net/http"
)

type healthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

func (api *API) live(writer http.ResponseWriter, _ *http.Request) {
	api.writeHealth(writer, http.StatusOK, "ok")
}

func (api *API) ready(writer http.ResponseWriter, request *http.Request) {
	if !api.readyState.Load() {
		writeError(writer, request, http.StatusServiceUnavailable, "NOT_READY", "Service is not ready.", true)
		return
	}
	if api.readiness != nil && api.readiness(request.Context()) != nil {
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required dependency is unavailable.", true)
		return
	}
	api.writeHealth(writer, http.StatusOK, "ok")
}

func (api *API) writeHealth(writer http.ResponseWriter, status int, health string) {
	writer.Header().Set("Content-Type", jsonContentType)
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(healthResponse{
		Status:    health,
		Service:   "platform-api",
		Version:   api.build.Version,
		Commit:    api.build.Commit,
		BuildTime: api.build.BuildTime,
	})
}
