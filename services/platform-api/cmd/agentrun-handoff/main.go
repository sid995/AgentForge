package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"

	kafkaadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/kafka"
	kubernetesadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/kubernetes"
	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	consumerapp "github.com/sid995/agentforge/services/platform-api/internal/application/consumer"
	handoffapp "github.com/sid995/agentforge/services/platform-api/internal/application/handoff"
	"github.com/sid995/agentforge/services/platform-api/internal/buildinfo"
	"github.com/sid995/agentforge/services/platform-api/internal/config"
	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/logging"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

var retryTopics = []string{
	"agentforge.agent-run.lifecycle.retry.1m.v1",
	"agentforge.agent-run.lifecycle.retry.5m.v1",
	"agentforge.agent-run.lifecycle.retry.30m.v1",
}

func main() {
	if err := run(); err != nil {
		slog.Error("AgentRun handoff stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configuration, err := config.LoadHandoff(os.LookupEnv)
	if err != nil {
		return err
	}
	kafkaConfiguration, err := config.LoadKafka(os.LookupEnv)
	if err != nil {
		return err
	}
	quarantineTenantID, err := uuid.Parse(configuration.QuarantineTenantID)
	if err != nil || quarantineTenantID.Version() != 7 {
		return fmt.Errorf("AGENTFORGE_HANDOFF_QUARANTINE_TENANT_ID must be a UUIDv7")
	}
	logger := logging.New(os.Stdout, configuration.Environment)
	startup, cancelStartup := context.WithTimeout(context.Background(), configuration.Database.ConnectTimeout)
	defer cancelStartup()
	pool, err := database.Open(startup, configuration.Database)
	if err != nil {
		return err
	}
	defer pool.Close()
	clientSelector, err := kubernetesadapter.NewClientSelector(configuration.KubeconfigPath, configuration.ClusterContexts)
	if err != nil {
		return err
	}
	repository := postgresadapter.NewHandoffRepository(pool)
	metrics := handoffapp.NewRuntimeMetrics()
	service, err := handoffapp.NewService(repository, clientSelector, repository, repository, metrics, logger)
	if err != nil {
		return err
	}
	topics := append([]string{events.AgentRunLifecycleTopic}, retryTopics...)
	consumer, err := kafkaadapter.NewConsumer(kafkaConfiguration, configuration.KafkaGroup, topics...)
	if err != nil {
		return err
	}
	defer consumer.Close()
	producer, err := kafkaadapter.NewProducer(kafkaConfiguration)
	if err != nil {
		return err
	}
	defer func() { _ = producer.Close() }()
	router, err := consumerapp.NewRouter(producer, metrics, handoffapp.ConsumerIdentity, quarantineTenantID)
	if err != nil {
		return err
	}

	processContext, cancelProcess := context.WithCancel(context.Background())
	defer cancelProcess()
	consumerDone := make(chan error, 1)
	go func() { consumerDone <- consume(processContext, consumer, service, router, logger) }()
	var ready atomic.Bool
	server := &http.Server{Handler: healthHandler(&ready, pool, metrics), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	listener, err := net.Listen("tcp", configuration.HTTPAddress)
	if err != nil {
		return err
	}
	ready.Store(true)
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	logger.Info("AgentRun handoff started", "address", listener.Addr().String(), "version", buildinfo.Version, "commit", buildinfo.Commit)

	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-consumerDone:
		return err
	case err := <-serveDone:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-signalContext.Done():
	}
	ready.Store(false)
	cancelProcess()
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), configuration.ShutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdown); err != nil {
		return err
	}
	select {
	case err := <-consumerDone:
		return err
	case <-shutdown.Done():
		return fmt.Errorf("AgentRun handoff shutdown timed out")
	}
}

func consume(ctx context.Context, consumer ports.EventConsumer, service *handoffapp.Service, router *consumerapp.Router, logger *slog.Logger) error {
	for {
		received, err := consumer.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var invalid *ports.InvalidEventError
			if errors.As(err, &invalid) {
				now := time.Now().UTC()
				if _, routeErr := router.Route(ctx, invalid.Record, consumerapp.NewPermanentFailure("INVALID_ENVELOPE", "invalid event envelope", err), malformedAttempt(invalid.Record.Topic), now, now); routeErr != nil {
					return routeErr
				}
				if acknowledgeErr := consumer.Acknowledge(ctx, invalid.Record); acknowledgeErr != nil {
					return acknowledgeErr
				}
				continue
			}
			return err
		}
		if received.Envelope.EventType != events.AgentRunScheduledType {
			if err := consumer.Acknowledge(ctx, received); err != nil {
				return err
			}
			continue
		}
		if notBefore, retryErr := retryNotBefore(received); retryErr != nil {
			return retryErr
		} else if !notBefore.IsZero() && time.Now().UTC().Before(notBefore) {
			timer := time.NewTimer(time.Until(notBefore))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
		now := time.Now().UTC()
		if err := service.Process(ctx, received, now); err != nil {
			attempt, attemptErr := deliveryAttempt(received)
			if attemptErr != nil {
				return attemptErr
			}
			routed, routeErr := router.Route(ctx, received, err, attempt, received.Envelope.OccurredAt, now)
			if routeErr != nil {
				return routeErr
			}
			logger.WarnContext(ctx, "AgentRun handoff event routed", "event_id", received.Envelope.EventID, "route", routed)
		}
		if err := consumer.Acknowledge(ctx, received); err != nil {
			return err
		}
	}
}

func malformedAttempt(topic string) int {
	for index, retryTopic := range retryTopics {
		if topic == retryTopic {
			return index + 2
		}
	}
	return 1
}

func retryNotBefore(event ports.ReceivedEvent) (time.Time, error) {
	if event.Topic == events.AgentRunLifecycleTopic {
		return time.Time{}, nil
	}
	return consumerapp.RetryNotBefore(event)
}

func deliveryAttempt(event ports.ReceivedEvent) (int, error) {
	if event.Topic == events.AgentRunLifecycleTopic {
		return 1, nil
	}
	for index, topic := range retryTopics {
		if event.Topic != topic {
			continue
		}
		for _, header := range event.Headers {
			if header.Key == "delivery-attempt" {
				previous, err := strconv.Atoi(header.Value)
				if err != nil || previous != index+1 {
					return 0, fmt.Errorf("delivery-attempt header is invalid")
				}
				return previous + 1, nil
			}
		}
	}
	return 0, fmt.Errorf("delivery attempt metadata is missing")
}

func healthHandler(ready *atomic.Bool, pool *database.Pool, metrics http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /health/ready", func(writer http.ResponseWriter, request *http.Request) {
		if !ready.Load() || pool.Ping(request.Context()) != nil {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
	})
	mux.Handle("GET /metrics", metrics)
	return mux
}
