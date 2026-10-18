package handler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pingFunc func(context.Context) error

func (f pingFunc) Ping(ctx context.Context) error { return f(ctx) }

func serveReady(t *testing.T, db Pinger) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	Ready(db).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
	return rec
}

func assertReadiness(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantBody string) {
	t.Helper()
	if rec.Code != wantStatus || strings.TrimSpace(rec.Body.String()) != wantBody {
		t.Fatalf("status=%d body=%q; want %d %s", rec.Code, rec.Body.String(), wantStatus, wantBody)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestHealthIsStatic(t *testing.T) {
	rec := httptest.NewRecorder()
	Health(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("status=%d body=%q; want 200 ok", rec.Code, rec.Body.String())
	}
}

func TestReadyWhenTheDatabaseAnswers(t *testing.T) {
	var deadline time.Time
	start := time.Now()
	rec := serveReady(t, pingFunc(func(ctx context.Context) error {
		deadline, _ = ctx.Deadline()
		return nil
	}))
	assertReadiness(t, rec, http.StatusOK, `{"status":"ok","db":"ok"}`)
	if deadline.IsZero() || deadline.After(start.Add(readyTimeout+time.Second)) {
		t.Fatalf("ping deadline = %v, want about %v after the request started", deadline, readyTimeout)
	}
}

func TestReadyWithTheMemoryStore(t *testing.T) {
	assertReadiness(t, serveReady(t, repository.NewMemoryStore(time.Now())), http.StatusOK, `{"status":"ok","db":"ok"}`)
}

func TestReadyIsDegradedWhenThePingFails(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	rec := serveReady(t, pingFunc(func(context.Context) error {
		return errors.New("password authentication failed for user secret-user")
	}))
	assertReadiness(t, rec, http.StatusServiceUnavailable, `{"status":"degraded","db":"error"}`)
	if !strings.Contains(logs.String(), "readiness check failed") || !strings.Contains(logs.String(), "secret-user") {
		t.Fatalf("logs = %q, want the ping error", logs.String())
	}
}

func TestReadyTracksAPostgresPool(t *testing.T) {
	databaseURL := os.Getenv("PERMITPAL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PERMITPAL_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	store := repository.NewPostgresStore(pool)
	assertReadiness(t, serveReady(t, store), http.StatusOK, `{"status":"ok","db":"ok"}`)
	pool.Close()
	assertReadiness(t, serveReady(t, store), http.StatusServiceUnavailable, `{"status":"degraded","db":"error"}`)
}

func TestReadyIsDegradedWhenPostgresIsUnreachable(t *testing.T) {
	// Reserve a local port, then close it so connections are refused.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(context.Background(), "postgres://permitpal:secret-password@"+addr+"/permitpal?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	rec := serveReady(t, repository.NewPostgresStore(pool))
	assertReadiness(t, rec, http.StatusServiceUnavailable, `{"status":"degraded","db":"error"}`)
}
