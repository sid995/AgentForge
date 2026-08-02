package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

type TrajectoryRecord struct {
	SchemaVersion int    `json:"schemaVersion"`
	Sequence      uint64 `json:"sequence"`
	Timestamp     string `json:"timestamp"`
	Type          string `json:"type"`
	Message       string `json:"message,omitempty"`
}

type TrajectorySink struct {
	mu       sync.Mutex
	sequence uint64
	writer   io.Writer
	redactor Redactor
	now      func() time.Time
}

func NewTrajectorySink(writer io.Writer, redactor Redactor) *TrajectorySink {
	return &TrajectorySink{writer: writer, redactor: redactor, now: time.Now}
}

func (sink *TrajectorySink) Emit(kind, message string) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.sequence++
	record := TrajectoryRecord{SchemaVersion: 1, Sequence: sink.sequence, Timestamp: sink.now().UTC().Format(time.RFC3339Nano), Type: kind, Message: sink.redactor.Redact(message)}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(sink.writer, "%s\n", encoded); err != nil {
		return err
	}
	return nil
}
