package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestAgentRunRequestedExampleMatchesMachineReadableSchema(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "..", ".."))
	schemaBytes, err := os.ReadFile(filepath.Join(root, "contracts", "events", "agent-run.requested.v1.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schemaDocument any
	if err := json.Unmarshal(schemaBytes, &schemaDocument); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("agent-run.requested.v1.schema.json", schemaDocument); err != nil {
		t.Fatalf("add schema: %v", err)
	}
	schema, err := compiler.Compile("agent-run.requested.v1.schema.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	exampleBytes, err := os.ReadFile(filepath.Join(root, "contracts", "events", "examples", "agent-run.requested.v1.json"))
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	var example any
	if err := json.Unmarshal(exampleBytes, &example); err != nil {
		t.Fatalf("decode example: %v", err)
	}
	if err := schema.Validate(example); err != nil {
		t.Fatalf("validate example: %v", err)
	}
	if _, err := DecodeEnvelope(exampleBytes); err != nil {
		t.Fatalf("runtime validation: %v", err)
	}
}

func TestDecodeEnvelopeAndHeadersRejectUnsupportedOrMismatchedContracts(t *testing.T) {
	event := mustRequestedEvent(t)
	headers := HeadersForEnvelope(event.Envelope)
	if err := ValidateHeaders(event.Envelope, headers); err != nil {
		t.Fatalf("valid headers: %v", err)
	}
	headers[0].Value = "019b0000-0000-7000-8000-000000000000"
	if err := ValidateHeaders(event.Envelope, headers); err == nil {
		t.Fatal("accepted mismatched headers")
	}
	envelope := event.Envelope
	envelope.SchemaVersion = 2
	if err := ValidateEnvelope(envelope); err == nil {
		t.Fatal("accepted unsupported schema version")
	}
	var object map[string]any
	if err := json.Unmarshal(event.Serialized, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown"] = true
	serialized, _ := json.Marshal(object)
	if _, err := DecodeEnvelope(serialized); err == nil {
		t.Fatal("accepted unknown envelope field")
	}
	headers = HeadersForEnvelope(event.Envelope)
	for len(headers) <= maxHeaderCount {
		headers = append(headers, Header{Key: "delivery-metadata", Value: "bounded"})
	}
	if err := ValidateHeaders(event.Envelope, headers); err == nil {
		t.Fatal("accepted excessive headers")
	}
}

func mustRequestedEvent(t *testing.T) OutboxEvent {
	t.Helper()
	// The typed constructor is already exercised in event_test; the checked-in
	// fixture keeps this contract test independent from domain setup.
	root := filepath.Clean(filepath.Join("..", "..", "..", ".."))
	serialized, err := os.ReadFile(filepath.Join(root, "contracts", "events", "examples", "agent-run.requested.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := DecodeEnvelope(serialized)
	if err != nil {
		t.Fatal(err)
	}
	return OutboxEvent{Envelope: envelope, Topic: AgentRunLifecycleTopic, PartitionKey: envelope.AggregateID.String(), Serialized: serialized, CreatedAt: envelope.OccurredAt}
}
