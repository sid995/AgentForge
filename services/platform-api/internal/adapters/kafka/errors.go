package kafka

import (
	"context"
	"errors"
	"io"
	"net"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// PublicationError classifies transport failures without exposing broker details.
type PublicationError struct {
	cause     error
	permanent bool
	reason    string
}

func (err *PublicationError) Error() string       { return err.reason }
func (err *PublicationError) Unwrap() error       { return err.cause }
func (err *PublicationError) Permanent() bool     { return err.permanent }
func (err *PublicationError) SafeMessage() string { return err.reason }

func classifyPublicationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &PublicationError{cause: err, reason: "Kafka publication timed out"}
	}
	for _, permanent := range []error{kerr.MessageTooLarge, kerr.TopicAuthorizationFailed, kerr.ClusterAuthorizationFailed, kerr.TransactionalIDAuthorizationFailed, kerr.UnknownTopicOrPartition, kerr.UnknownTopicID} {
		if errors.Is(err, permanent) {
			return &PublicationError{cause: err, permanent: true, reason: "Kafka rejected the event"}
		}
	}
	var networkError net.Error
	if errors.As(err, &networkError) || errors.Is(err, io.EOF) || errors.Is(err, kgo.ErrRecordTimeout) || errors.Is(err, kgo.ErrRecordRetries) {
		return &PublicationError{cause: err, reason: "Kafka is temporarily unavailable"}
	}
	if kerr.IsRetriable(err) {
		return &PublicationError{cause: err, reason: "Kafka is temporarily unavailable"}
	}
	return &PublicationError{cause: err, permanent: true, reason: "Kafka publication failed permanently"}
}
