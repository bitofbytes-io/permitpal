package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/config"
)

const testUserHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

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

func TestRunReturnsErrorWhenPortIsTaken(t *testing.T) {
	held, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Close() })
	setDevEnv(t, strconv.Itoa(held.Addr().(*net.TCPAddr).Port), "off")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = run(ctx)
	if err == nil || !strings.Contains(err.Error(), "server failed") || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("run error = %v, want listen failure", err)
	}
}

func TestRunReleasesListenersWhenCanceled(t *testing.T) {
	port := freePort(t, ":0")
	metricsPort := freePort(t, "127.0.0.1:0")
	setDevEnv(t, port, "127.0.0.1:"+metricsPort)

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	var runErr error
	go func() {
		runErr = run(ctx)
		close(finished)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(15 * time.Second):
			t.Errorf("run did not return after cancel")
		}
	})

	waitForOK(t, finished, "http://127.0.0.1:"+port+"/health")
	waitForOK(t, finished, "http://127.0.0.1:"+metricsPort+"/metrics")
	cancel()

	select {
	case <-finished:
	case <-time.After(15 * time.Second):
		t.Fatal("run did not return")
	}
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	mustListen(t, ":"+port)
	mustListen(t, "127.0.0.1:"+metricsPort)
}

func setDevEnv(t *testing.T, port, metricsAddr string) {
	t.Helper()
	logger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(logger) })
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATA_STORE", "memory")
	t.Setenv("SECURE_COOKIES", "false")
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("PORT", port)
	t.Setenv("METRICS_ADDR", metricsAddr)
	t.Setenv("PERMITPAL_USERS", "driver:"+testUserHash)
	t.Setenv("SESSION_SECRET", "local-session-secret-32-bytes-ok")
}

func freePort(t *testing.T, addr string) string {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func waitForOK(t *testing.T, finished <-chan struct{}, url string) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		select {
		case <-finished:
			t.Fatalf("run returned before %s responded: %v", url, last)
		default:
		}
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			last = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("GET %s: %v", url, last)
}

func mustListen(t *testing.T, addr string) {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen %s: %v", addr, err)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
}
