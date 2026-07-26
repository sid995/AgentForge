package httpapi

import (
	"net/http"
	"strings"

	"github.com/sid995/agentforge/services/platform-api/internal/identity"
)

func (api *API) authenticate(writer http.ResponseWriter, request *http.Request) (identity.Identity, bool) {
	if api.identityResolver == nil {
		writeError(writer, request, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "Authentication is required.", false)
		return identity.Identity{}, false
	}
	credential, ok := bearerCredential(request.Header.Get("Authorization"))
	if !ok {
		writeError(writer, request, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "Authentication is required.", false)
		return identity.Identity{}, false
	}
	caller, err := api.identityResolver.Resolve(request.Context(), credential)
	if err != nil {
		writeError(writer, request, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "Authentication is required.", false)
		return identity.Identity{}, false
	}
	return caller, true
}

func bearerCredential(value string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	credential := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	return credential, credential != ""
}
