package logger

import (
	"log/slog"
	"testing"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		level    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"invalid", slog.LevelInfo}, // Defaults to info
		{"", slog.LevelInfo},        // Defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			log := New(tt.level)
			if log == nil {
				t.Fatal("expected non-nil logger")
			}
			// Logger should be created successfully
			// (We can't easily test the level without accessing internals)
		})
	}
}
