package loggertest

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"ledgerline/internal/shared/logger"
)

// captureStdout redirects os.Stdout while the logger is built and used, and
// returns what was written. The handler captures os.Stdout when it is
// created, so the logger has to be built inside the redirect.
func captureStdout(t *testing.T, level, env string, fn func(log *slog.Logger)) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	fn(logger.New(level, env))

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
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
			if log == nil {
				t.Fatal("New() = nil")
			}
			if !log.Enabled(t.Context(), tt.wantLog) {
				t.Errorf("level %q is not enabled", tt.level)
			}
		})
	}
}

// TestNew_UnknownLevelFallsBackToInfo checks that a level the parser does not
// understand does not disable logging.
func TestNew_UnknownLevelFallsBackToInfo(t *testing.T) {
	log := logger.New("not-a-level", "development")

	if log == nil {
		t.Fatal("New() = nil")
	}
	if !log.Enabled(t.Context(), slog.LevelInfo) {
		t.Error("an unknown level should fall back to info")
	}
}

// TestNew_LevelFiltersMessages checks that the configured level is applied.
func TestNew_LevelFiltersMessages(t *testing.T) {
	out := captureStdout(t, "error", "development", func(log *slog.Logger) {
		log.Info("should not appear")
		log.Error("should appear")
	})

	if strings.Contains(out, "should not appear") {
		t.Errorf("output = %q, want the info message to be filtered out", out)
	}
	if !strings.Contains(out, "should appear") {
		t.Errorf("output = %q, want it to contain the error message", out)
	}
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

			if !strings.Contains(out, tt.wantIn) {
				t.Errorf("output = %q, want it to contain %q", out, tt.wantIn)
			}
		})
	}
}
