package main

import (
	"context"
	"encoding/json"
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

	kafkaadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/kafka"
	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	schedulerapp "github.com/sid995/agentforge/services/platform-api/internal/application/scheduler"
	"github.com/sid995/agentforge/services/platform-api/internal/buildinfo"
	"github.com/sid995/agentforge/services/platform-api/internal/config"
	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/logging"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func main() {
	if err := run(); err != nil {
		slog.Error("scheduler stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configuration, err := config.LoadScheduler(os.LookupEnv)
	if err != nil {
		return err
	}
	if configuration.Owner == "" {
		hostname, _ := os.Hostname()
		configuration.Owner = hostname + "-" + strconv.Itoa(os.Getpid())
	}
	logger := logging.New(os.Stdout, configuration.Environment)
	startup, cancelStartup := context.WithTimeout(context.Background(), configuration.Database.ConnectTimeout)
	defer cancelStartup()
	pool, err := database.Open(startup, configuration.Database)
	if err != nil {
		return err
	}
	defer pool.Close()
	metrics := schedulerapp.NewMetrics()
	strategy, err := schedulerapp.NewStrategy(configuration.Strategy)
	if err != nil {
		return err
	}
	queue := postgresadapter.NewSchedulerQueueRepository(pool)
	engine, err := schedulerapp.NewEngine(schedulerapp.EngineConfig{Owner: configuration.Owner, PollInterval: configuration.PollInterval, LeaseDuration: configuration.LeaseDuration, BatchSize: configuration.BatchSize, WorkerCount: configuration.WorkerCount, WorkQueueSize: configuration.WorkQueueSize, AgingInterval: configuration.AgingInterval, MaximumAgingBoost: configuration.MaximumAgingBoost, ClusterFreshness: configuration.ClusterFreshness, ReservationTTL: configuration.ReservationTTL, DeferralDuration: configuration.DeferralDuration}, schedulerapp.NewClaimer(queue, metrics, logger), schedulerapp.NewEligibilityService(postgresadapter.NewSchedulerEligibilityRepository(pool), metrics, logger), postgresadapter.NewClusterRegistry(pool), schedulerapp.NewSelector(strategy, logger, metrics), postgresadapter.NewSchedulingIntentRepository(pool), metrics, logger)
	if err != nil {
		return err
	}
	processContext, cancelProcess := context.WithCancel(context.Background())
	defer cancelProcess()
	engineDone := make(chan error, 1)
	go func() { engineDone <- engine.Run(processContext) }()
	var hintConsumer *kafkaadapter.Consumer
	if configuration.KafkaHintsEnabled {
		kafkaConfiguration, loadErr := config.LoadKafka(os.LookupEnv)
		if loadErr != nil {
			return loadErr
		}
		hintConsumer, err = kafkaadapter.NewConsumer(kafkaConfiguration, configuration.KafkaGroup, events.AgentRunLifecycleTopic)
		if err != nil {
			return err
		}
		defer hintConsumer.Close()
		go runHints(processContext, hintConsumer, engine, logger)
	}
	var ready atomic.Bool
	readiness := func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return err
		}
		var migrated bool
		if err := pool.Raw().QueryRowContext(ctx, `select to_regclass('public.agent_runs') is not null and to_regclass('public.capacity_reservations') is not null`).Scan(&migrated); err != nil {
			return err
		}
		if !migrated {
			return fmt.Errorf("scheduler schema is unavailable")
		}
		return nil
	}
	mux := newServiceHandler(&ready, readiness, metrics)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	listener, err := net.Listen("tcp", configuration.HTTPAddress)
	if err != nil {
		return err
	}
	ready.Store(true)
	logger.Info("scheduler started", "address", listener.Addr().String(), "owner", configuration.Owner, "workers", configuration.WorkerCount, "version", buildinfo.Version, "commit", buildinfo.Commit)
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serveDone:
		if !errors.Is(err, http.ErrServerClosed) {
			cancelProcess()
			return err
		}
		return nil
	case err := <-engineDone:
		if err != nil {
			return err
		}
		return fmt.Errorf("scheduler engine stopped unexpectedly")
	case <-signalContext.Done():
	}
	ready.Store(false)
	cancelProcess()
	logger.Info("scheduler shutting down")
	shutdown, cancel := context.WithTimeout(context.Background(), configuration.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		return err
	}
	select {
	case err := <-engineDone:
		if err != nil {
			return err
		}
	case <-shutdown.Done():
		return fmt.Errorf("scheduler shutdown timed out")
	}
	if err := <-serveDone; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runHints(ctx context.Context, consumer ports.EventConsumer, engine *schedulerapp.Engine, logger *slog.Logger) {
	for {
		received, err := consumer.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			var invalid *ports.InvalidEventError
			if errors.As(err, &invalid) {
				if acknowledgeErr := consumer.Acknowledge(ctx, invalid.Record); acknowledgeErr == nil {
					logger.WarnContext(ctx, "scheduler discarded invalid optional wake hint", "error", "invalid hint")
					continue
				}
			}
			logger.WarnContext(ctx, "scheduler Kafka hint unavailable", "error", "hint poll failed")
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		if received.Envelope.EventType == events.AgentRunRequestedType {
			engine.Wake()
			logger.InfoContext(ctx, "scheduler wake hint received", "correlation_id", received.Envelope.CorrelationID, "traceparent", received.Envelope.Traceparent)
		}
		if err := consumer.Acknowledge(ctx, received); err != nil && ctx.Err() == nil {
			logger.WarnContext(ctx, "scheduler Kafka hint acknowledgement failed", "error", "hint acknowledgement failed")
		}
	}
}

func writeHealth(writer http.ResponseWriter, status int, state string) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"status": state, "service": "scheduler", "version": buildinfo.Version, "commit": buildinfo.Commit, "buildTime": buildinfo.BuildTime})
}

func newServiceHandler(ready *atomic.Bool, readiness func(context.Context) error, metrics http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(writer http.ResponseWriter, _ *http.Request) { writeHealth(writer, http.StatusOK, "ok") })
	mux.HandleFunc("GET /health/ready", func(writer http.ResponseWriter, request *http.Request) {
		if !ready.Load() || readiness(request.Context()) != nil {
			writeHealth(writer, http.StatusServiceUnavailable, "not_ready")
			return
		}
		writeHealth(writer, http.StatusOK, "ok")
	})
	mux.Handle("GET /metrics", metrics)
	return mux
}
