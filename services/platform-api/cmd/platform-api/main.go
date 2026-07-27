package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	"github.com/sid995/agentforge/services/platform-api/internal/application/runs"
	"github.com/sid995/agentforge/services/platform-api/internal/buildinfo"
	"github.com/sid995/agentforge/services/platform-api/internal/config"
	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/httpapi"
	"github.com/sid995/agentforge/services/platform-api/internal/identity"
	"github.com/sid995/agentforge/services/platform-api/internal/logging"
)

func main() {
	if err := run(); err != nil {
		slog.Error("platform-api stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configuration, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, configuration.Environment)
	startupContext, cancelStartup := context.WithTimeout(context.Background(), configuration.Database.ConnectTimeout)
	defer cancelStartup()
	databasePool, err := database.Open(startupContext, configuration.Database)
	if err != nil {
		return err
	}
	defer databasePool.Close()
	var resolver identity.Resolver
	if configuration.DevelopmentIdentity != nil {
		resolver, err = identity.NewDevelopmentResolver([]identity.DevelopmentCredential{{
			Token:    configuration.DevelopmentIdentity.Token,
			Identity: identity.Identity{TenantID: configuration.DevelopmentIdentity.TenantID, Subject: configuration.DevelopmentIdentity.Subject, Role: identity.Role(configuration.DevelopmentIdentity.Role)},
		}})
		if err != nil {
			return err
		}
	}
	runService := runs.NewService(postgres.NewAgentRunRepository(databasePool), postgres.NewProjectRepository(databasePool), nil)
	api := httpapi.New(httpapi.Options{
		Logger:              logger,
		Build:               buildinfo.Current(),
		RequestTimeout:      configuration.RequestTimeout,
		MaxRequestBodyBytes: configuration.MaxRequestBodyBytes,
		Readiness:           databasePool.Ping,
		IdentityResolver:    resolver,
		Runs:                runService,
	})
	server := &http.Server{
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	listener, err := net.Listen("tcp", configuration.HTTPAddress)
	if err != nil {
		return err
	}
	api.SetReady(true)
	logger.Info("platform-api started", "address", listener.Addr().String(), "version", buildinfo.Version, "commit", buildinfo.Commit)

	serveError := make(chan error, 1)
	go func() { serveError <- server.Serve(listener) }()

	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serveError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-signalContext.Done():
	}

	api.SetReady(false)
	logger.Info("platform-api shutting down")
	shutdownContext, cancel := context.WithTimeout(context.Background(), configuration.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return err
	}
	if err := <-serveError; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
