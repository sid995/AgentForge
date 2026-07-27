package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sid995/agentforge/agent-runner/internal/runner"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	code, err := runner.Run(ctx, runner.Options{ConfigPath: os.Getenv("AGENTFORGE_RUN_CONFIG"), Workspace: os.Getenv("AGENTFORGE_WORKSPACE"), LogWriter: os.Stdout})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if errors.Is(err, context.Canceled) {
		code = runner.ExitCancelled
	}
	os.Exit(code)
}
