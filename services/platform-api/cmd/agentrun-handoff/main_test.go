package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type topicTestConsumer struct{ closed bool }

func (*topicTestConsumer) Poll(context.Context) (ports.ReceivedEvent, error) {
	return ports.ReceivedEvent{}, errors.New("not implemented")
}

func (*topicTestConsumer) Acknowledge(context.Context, ports.ReceivedEvent) error {
	return errors.New("not implemented")
}

func (consumer *topicTestConsumer) Close() { consumer.closed = true }

func TestOpenTopicConsumersIsolatesPrimaryAndRetryDelays(t *testing.T) {
	topics := append([]string{events.AgentRunLifecycleTopic}, retryTopics...)
	var openedTopics []string
	consumers, err := openTopicConsumers(topics, func(topic string) (ports.EventConsumer, error) {
		openedTopics = append(openedTopics, topic)
		return &topicTestConsumer{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(consumers) != len(topics) || !reflect.DeepEqual(openedTopics, topics) {
		t.Fatalf("consumers=%d topics=%v, want one consumer for each %v", len(consumers), openedTopics, topics)
	}
}

func TestOpenTopicConsumersClosesPartialSetOnFailure(t *testing.T) {
	first := &topicTestConsumer{}
	_, err := openTopicConsumers([]string{"first", "second"}, func(topic string) (ports.EventConsumer, error) {
		if topic == "second" {
			return nil, errors.New("open failed")
		}
		return first, nil
	})
	if err == nil || !first.closed {
		t.Fatalf("error=%v first.closed=%v", err, first.closed)
	}
}
