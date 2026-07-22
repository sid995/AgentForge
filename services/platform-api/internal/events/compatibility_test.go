package events

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

type compatibilityBaseline struct {
	Events []struct {
		Schema   string              `json:"schema"`
		Required map[string][]string `json:"required"`
		Fields   map[string]string   `json:"fields"`
	} `json:"events"`
}

func TestEventContractSchemasExamplesAndRuntimeDecoders(t *testing.T) {
	root := contractRoot()
	schemas, err := filepath.Glob(filepath.Join(root, "*.schema.json"))
	if err != nil || len(schemas) == 0 {
		t.Fatalf("discover event schemas: %v", err)
	}
	for _, schemaPath := range schemas {
		name := strings.TrimSuffix(filepath.Base(schemaPath), ".schema.json")
		t.Run(name, func(t *testing.T) {
			examplePath := filepath.Join(root, "examples", name+".json")
			example := readJSONDocument(t, examplePath)
			if err := compileSchema(t, schemaPath).Validate(example); err != nil {
				t.Fatalf("schema rejected canonical example: %v", err)
			}
			serialized, err := os.ReadFile(examplePath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeEnvelope(serialized); err != nil {
				t.Fatalf("runtime decoder rejected canonical example: %v", err)
			}
		})
	}
}

func TestEventContractCompatibilityBaseline(t *testing.T) {
	root := contractRoot()
	var baseline compatibilityBaseline
	decodeJSONFile(t, filepath.Join(root, "compatibility", "v1.json"), &baseline)
	if len(baseline.Events) == 0 {
		t.Fatal("compatibility baseline contains no events")
	}
	for _, event := range baseline.Events {
		t.Run(event.Schema, func(t *testing.T) {
			current := readJSONDocument(t, filepath.Join(root, event.Schema))
			for pointer, required := range event.Required {
				actual, ok := jsonPointer(current, pointer).([]any)
				if !ok {
					t.Fatalf("required array %s is missing", pointer)
				}
				actualStrings := make([]string, len(actual))
				for index, value := range actual {
					actualStrings[index], ok = value.(string)
					if !ok {
						t.Fatalf("required array %s contains a non-string", pointer)
					}
				}
				sort.Strings(actualStrings)
				if !reflect.DeepEqual(actualStrings, required) {
					t.Fatalf("required fields changed at %s: got %v want %v; publish a new major version", pointer, actualStrings, required)
				}
			}
			for pointer, want := range event.Fields {
				node, ok := jsonPointer(current, pointer).(map[string]any)
				if !ok {
					t.Fatalf("published field removed at %s; publish a new major version", pointer)
				}
				if got := fieldSignature(node); got != want {
					t.Fatalf("published field changed incompatibly at %s: got %q want %q; publish a new major version", pointer, got, want)
				}
			}
		})
	}
}

func TestEventContractProducerOutputMatchesSchemas(t *testing.T) {
	requested := mustRequestedEvent(t)
	validateSerializedContract(t, "agent-run.requested.v1", requested.Serialized)
	var requestedPayload AgentRunRequestedPayload
	if err := json.Unmarshal(requested.Envelope.Payload, &requestedPayload); err != nil {
		t.Fatal(err)
	}
	run := domain.AgentRun{ID: *requested.Envelope.RunID, TenantID: requested.Envelope.TenantID, ProjectID: *requested.Envelope.ProjectID, Status: domain.AgentRunProvisioning, Version: 2, CPUMillis: requestedPayload.CPUMillis, MemoryMiB: requestedPayload.MemoryMiB, UpdatedAt: requested.Envelope.OccurredAt.Add(time.Second)}
	attempt := domain.AgentRunAttempt{ID: mustV7(t), TenantID: run.TenantID, RunID: run.ID, AttemptNumber: 1, Status: domain.AttemptStarting, Version: 2, SelectedCluster: "cluster-east", ExecutionProfile: "standard"}
	scheduled, err := NewAgentRunScheduled(run, attempt, mustV7(t), mustV7(t), "correlation", "causation")
	if err != nil {
		t.Fatal(err)
	}
	validateSerializedContract(t, "agent-run.scheduled.v1", scheduled.Serialized)
	waitEvent, err := NewAgentRunCapacityWait(domain.AgentRun{ID: run.ID, TenantID: run.TenantID, ProjectID: run.ProjectID, Status: domain.AgentRunCapacityWait, AttemptCount: 1, Version: 2, UpdatedAt: run.UpdatedAt}, "NO_CAPACITY", "no eligible cluster has sufficient capacity", run.UpdatedAt.Add(time.Minute), "correlation", "causation")
	if err != nil {
		t.Fatal(err)
	}
	validateSerializedContract(t, "agent-run.capacity-wait.v1", waitEvent.Serialized)
	now := time.Date(2026, 7, 21, 14, 0, 0, 0, time.UTC)
	dlq, err := NewDeadLetter(DeadLetterSource{Envelope: requested.Envelope, Topic: requested.Topic, Partition: 1, Offset: 9, Key: []byte(requested.PartitionKey), Headers: HeadersForEnvelope(requested.Envelope), Value: requested.Serialized}, "scheduler.v1", "INVARIANT_FAILED", "event violates scheduler invariant", 1, now, now)
	if err != nil {
		t.Fatalf("construct producer DLQ event: %v", err)
	}
	validateSerializedContract(t, "event-delivery.dead-lettered.v1", dlq.Serialized)
}

func mustV7(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestEventContractSupportedConsumerFixtures(t *testing.T) {
	root := contractRoot()
	fixtures, err := filepath.Glob(filepath.Join(root, "fixtures", "consumer", "v1", "*.json"))
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("discover supported consumer fixtures: %v", err)
	}
	for _, fixturePath := range fixtures {
		name := strings.TrimSuffix(filepath.Base(fixturePath), ".json")
		t.Run(name, func(t *testing.T) {
			serialized, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatal(err)
			}
			validateSerializedContract(t, name, serialized)
			if _, err := DecodeEnvelope(serialized); err != nil {
				t.Fatalf("supported v1 fixture rejected by consumer: %v", err)
			}
		})
	}
}

func contractRoot() string {
	return filepath.Clean(filepath.Join("..", "..", "..", "..", "contracts", "events"))
}

func validateSerializedContract(t *testing.T, name string, serialized []byte) {
	t.Helper()
	var document any
	if err := json.Unmarshal(serialized, &document); err != nil {
		t.Fatalf("decode %s contract: %v", name, err)
	}
	if err := compileSchema(t, filepath.Join(contractRoot(), name+".schema.json")).Validate(document); err != nil {
		t.Fatalf("validate %s contract: %v", name, err)
	}
}

func compileSchema(t *testing.T, path string) *jsonschema.Schema {
	t.Helper()
	document := readJSONDocument(t, path)
	compiler := jsonschema.NewCompiler()
	name := filepath.Base(path)
	if err := compiler.AddResource(name, document); err != nil {
		t.Fatalf("add schema %s: %v", name, err)
	}
	schema, err := compiler.Compile(name)
	if err != nil {
		t.Fatalf("compile schema %s: %v", name, err)
	}
	return schema
}

func readJSONDocument(t *testing.T, path string) map[string]any {
	t.Helper()
	var document map[string]any
	decodeJSONFile(t, path, &document)
	return document
}

func decodeJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	serialized, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(serialized, target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func jsonPointer(document any, pointer string) any {
	current := document
	for _, token := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")]
	}
	return current
}

func fieldSignature(node map[string]any) string {
	if reference, ok := node["$ref"].(string); ok {
		return "ref:" + reference
	}
	if fieldType, ok := node["type"].(string); ok {
		return "type:" + fieldType
	}
	if constant, ok := node["const"]; ok {
		kind := "number"
		if _, stringConstant := constant.(string); stringConstant {
			kind = "string"
		}
		return fmt.Sprintf("const:%s:%v", kind, constant)
	}
	return "unknown"
}
