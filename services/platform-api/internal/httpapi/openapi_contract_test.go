package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAPIContractDocumentsImplementedRunRoutes(t *testing.T) {
	path, err := filepath.Abs("../../../../contracts/openapi/platform-api.v1.json")
	if err != nil {
		t.Fatalf("resolve OpenAPI contract: %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	var contract struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(payload, &contract); err != nil {
		t.Fatalf("decode OpenAPI contract: %v", err)
	}
	if contract.OpenAPI != "3.1.1" {
		t.Fatalf("OpenAPI version=%q", contract.OpenAPI)
	}
	for _, path := range []string{"/v1/projects/{projectId}/runs", "/v1/runs/{runId}", "/v1/runs/{runId}/cancel", "/v1/runs/{runId}/retry"} {
		if _, ok := contract.Paths[path]; !ok {
			t.Fatalf("OpenAPI contract is missing %s", path)
		}
	}
}
