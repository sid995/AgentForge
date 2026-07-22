package ports

import (
	"context"
	"fmt"

	"github.com/sid995/agentforge/services/platform-api/internal/events"
)

// ReceivedEvent is a validated broker record with provider-neutral source metadata.
type ReceivedEvent struct {
	Envelope    events.Envelope
	Topic       string
	Partition   int32
	LeaderEpoch int32
	Offset      int64
	Key         []byte
	Headers     []events.Header
	Value       []byte
}

// InvalidEventError preserves bounded source metadata for deliberate DLQ handling.
type InvalidEventError struct {
	Record ReceivedEvent
	Cause  error
}

func (err *InvalidEventError) Error() string {
	return fmt.Sprintf("invalid broker event: %v", err.Cause)
}
func (err *InvalidEventError) Unwrap() error       { return err.Cause }
func (err *InvalidEventError) Permanent() bool     { return true }
func (err *InvalidEventError) SafeMessage() string { return "invalid broker event" }

// EventConsumer polls explicitly and never acknowledges before its caller commits.
type EventConsumer interface {
	Poll(context.Context) (ReceivedEvent, error)
	Acknowledge(context.Context, ReceivedEvent) error
	Close()
}
