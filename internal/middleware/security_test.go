package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireSameOriginRejectsSchemeMismatch(t *testing.T) {
	called := false
	handler := RequireSameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "https://permitpal.example/requirements/backing", nil)
	req.Host = "permitpal.example"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Origin", "http://permitpal.example")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("handler was called for cross-scheme request")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireSameOriginRejectsPortMismatch(t *testing.T) {
	called := false
	handler := RequireSameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "https://permitpal.example:8443/requirements/backing", nil)
	req.Host = "permitpal.example:8443"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Origin", "https://permitpal.example")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("handler was called for cross-port request")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireSameOriginRejectsHostMismatch(t *testing.T) {
	called := false
	handler := RequireSameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "https://permitpal.example/requirements/backing", nil)
	req.Host = "permitpal.example"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Origin", "https://attacker.example")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("handler was called for cross-host request")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireSameOriginAcceptsMatchingForwardedSchemeHostAndPort(t *testing.T) {
	handler := RequireSameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://permitpal.example:4600/requirements/backing", nil)
	req.Host = "permitpal.example:4600"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Origin", "https://permitpal.example:4600")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRequireSameOriginIgnoresInvalidForwardedProto(t *testing.T) {
	handler := RequireSameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://permitpal.example/requirements/backing", nil)
	req.Host = "permitpal.example"
	req.Header.Set("X-Forwarded-Proto", "gopher")
	req.Header.Set("Origin", "http://permitpal.example")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestLoggerEmitsDebugAccessLog(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	logs := buf.String()
	if !strings.Contains(logs, "http request") {
		t.Fatalf("log output = %q, want request log", logs)
	}
	if !strings.Contains(logs, "path=/health") {
		t.Fatalf("log output = %q, want path", logs)
	}
	if !strings.Contains(logs, "status=204") {
		t.Fatalf("log output = %q, want status", logs)
	}
}
