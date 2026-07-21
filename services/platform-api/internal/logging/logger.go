package logging

import (
	"io"
	"log/slog"
)

// New returns the JSON logger shared by Platform API components.
func New(output io.Writer, environment string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{})).With(
		slog.String("service", "platform-api"),
		slog.String("environment", environment),
	)
}
