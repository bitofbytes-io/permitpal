package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/config"
)

func TestSlogLevel(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want slog.Level
	}{
		{name: "nil config", cfg: nil, want: slog.LevelInfo},
		{name: "debug", cfg: &config.Config{LogLevel: "debug"}, want: slog.LevelDebug},
		{name: "warn", cfg: &config.Config{LogLevel: "warn"}, want: slog.LevelWarn},
		{name: "error", cfg: &config.Config{LogLevel: "error"}, want: slog.LevelError},
		{name: "info", cfg: &config.Config{LogLevel: "info"}, want: slog.LevelInfo},
		{name: "empty", cfg: &config.Config{}, want: slog.LevelInfo},
		{name: "unknown", cfg: &config.Config{LogLevel: "verbose"}, want: slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := slogLevel(tt.cfg); got != tt.want {
				t.Fatalf("slogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunReturnsErrorOnInvalidConfig(t *testing.T) {
	t.Setenv("APP_ENV", "not-a-real-env")
	logger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(logger) })

	// A timeout stops the test hanging if invalid config ever falls through to the server.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := run(ctx)
	if err == nil {
		t.Fatal("run returned nil error")
	}
	if !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("run error = %v, want APP_ENV validation error", err)
	}
}
