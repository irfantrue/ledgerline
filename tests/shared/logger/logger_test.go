package loggertest

import (
	"bytes"
	"log/slog"
	"os"
	"testing"

	"ledgerline/internal/shared/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdout redirects os.Stdout while the logger is built and used, and
// returns what was written. The handler captures os.Stdout when it is
// created, so the logger has to be built inside the redirect.
func captureStdout(t *testing.T, level, env string, fn func(log *slog.Logger)) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err, "create pipe")

	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	fn(logger.New(level, env))

	require.NoError(t, w.Close(), "close pipe")

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err, "read pipe")
	_ = r.Close()

	return buf.String()
}

func TestNew_Level(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		wantLog slog.Level
	}{
		{name: "debug", level: "debug", wantLog: slog.LevelDebug},
		{name: "info", level: "info", wantLog: slog.LevelInfo},
		{name: "warn", level: "warn", wantLog: slog.LevelWarn},
		{name: "error", level: "error", wantLog: slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logger.New(tt.level, "development")
			require.NotNil(t, log, "New() should return a logger")

			assert.True(t, log.Enabled(t.Context(), tt.wantLog), "level %q should be enabled", tt.level)
		})
	}
}

// TestNew_UnknownLevelFallsBackToInfo checks that a level the parser does not
// understand does not disable logging.
func TestNew_UnknownLevelFallsBackToInfo(t *testing.T) {
	log := logger.New("not-a-level", "development")

	require.NotNil(t, log, "New() should return a logger")

	assert.True(t, log.Enabled(t.Context(), slog.LevelInfo), "an unknown level should fall back to info")
}

// TestNew_LevelFiltersMessages checks that the configured level is applied.
func TestNew_LevelFiltersMessages(t *testing.T) {
	out := captureStdout(t, "error", "development", func(log *slog.Logger) {
		log.Info("should not appear")
		log.Error("should appear")
	})

	assert.NotContains(t, out, "should not appear", "the info message should be filtered out")
	assert.Contains(t, out, "should appear", "the error message should be logged")
}

// TestNew_Format checks the JSON output used in production and the text
// output used everywhere else.
func TestNew_Format(t *testing.T) {
	tests := []struct {
		name   string
		env    string
		wantIn string
	}{
		{name: "production writes json", env: "production", wantIn: `"msg":"hello"`},
		{name: "development writes text", env: "development", wantIn: "msg=hello"},
		{name: "staging writes text", env: "staging", wantIn: "msg=hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, "info", tt.env, func(log *slog.Logger) {
				log.Info("hello")
			})

			assert.Contains(t, out, tt.wantIn, "output should contain the expected message format")
		})
	}
}
