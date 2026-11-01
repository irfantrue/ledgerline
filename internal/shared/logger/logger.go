package logger

import (
	"log/slog"
	"os"
)

// New creates a new slog.Logger with the specified log level and environment.
// In production mode, it uses JSON output format; otherwise text format.
func New(level, env string) *slog.Logger {
	var lvl slog.LevelVar
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl.Set(slog.LevelInfo)
	}

	opts := &slog.HandlerOptions{Level: &lvl}
	if env == "production" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}

	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
