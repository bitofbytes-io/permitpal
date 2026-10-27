package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

func TestLoggerRecordsSuccessFields(t *testing.T) {
	t.Run("written response", func(t *testing.T) {
		buf := captureLogs(t, slog.LevelDebug)
		const body = "ok"
		handler := chimw.RequestID(Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(body)); err != nil {
				t.Errorf("write body: %v", err)
			}
		})))
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set(chimw.RequestIDHeader, "req-success")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || rec.Body.String() != body {
			t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
		}
		record := logRecord(t, buf)
		if record["level"] != "DEBUG" || record["msg"] != "http request" {
			t.Fatalf("record = %#v", record)
		}
		assertStringField(t, record, "method", http.MethodGet)
		assertStringField(t, record, "path", "/health")
		assertStringField(t, record, "request_id", "req-success")
		assertNumberField(t, record, "status", http.StatusOK)
		assertNumberField(t, record, "bytes", len(body))
		assertDurationField(t, record)
	})

	t.Run("unwritten response defaults status to 200", func(t *testing.T) {
		buf := captureLogs(t, slog.LevelDebug)
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()

		Logger(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, req)

		record := logRecord(t, buf)
		if record["level"] != "DEBUG" {
			t.Fatalf("record = %#v", record)
		}
		assertStringField(t, record, "path", "/health")
		assertNumberField(t, record, "status", http.StatusOK)
		assertNumberField(t, record, "bytes", 0)
		assertDurationField(t, record)
	})
}

func TestLoggerRecordsErrorStatus(t *testing.T) {
	tests := []struct {
		name  string
		code  int
		level string
	}{
		{name: "client error", code: http.StatusBadRequest, level: "WARN"},
		{name: "server error", code: http.StatusInternalServerError, level: "ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLogs(t, slog.LevelInfo)
			const message = "nope"
			handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, message, tt.code)
			}))
			req := httptest.NewRequest(http.MethodPost, "/requirements/7", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.code {
				t.Fatalf("status = %d, want %d", rec.Code, tt.code)
			}
			record := logRecord(t, buf)
			if record["level"] != tt.level || record["msg"] != "http request" {
				t.Fatalf("record = %#v", record)
			}
			assertStringField(t, record, "method", http.MethodPost)
			assertStringField(t, record, "path", "/requirements/7")
			assertNumberField(t, record, "status", tt.code)
			assertNumberField(t, record, "bytes", len(message)+len("\n"))
			assertDurationField(t, record)
		})
	}
}

func TestLoggerOmitsQueryAndCredentials(t *testing.T) {
	const (
		path         = "/session"
		queryToken   = "query-token-9f3a"
		queryPass    = "query-password-9f3a"
		userName     = "basic-user-9f3a"
		userPassword = "userinfo-password-9f3a"
		bearer       = "header-bearer-9f3a"
		apiKey       = "header-api-key-9f3a"
		cookieValue  = "cookie-session-9f3a"
		bodyPassword = "body-password-9f3a"
	)
	buf := captureLogs(t, slog.LevelDebug)
	target := "http://" + userName + ":" + userPassword + "@permitpal.example" + path + "?token=" + queryToken + "&password=" + queryPass
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader("password="+bodyPassword))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("X-Api-Key", apiKey)
	req.AddCookie(&http.Cookie{Name: "session", Value: cookieValue})
	if req.URL.Path != path || req.URL.RawQuery == "" || req.URL.User == nil {
		t.Fatalf("fixture lost path, query, or userinfo: %#v", req.URL)
	}
	rec := httptest.NewRecorder()

	Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	record := logRecord(t, buf)
	assertStringField(t, record, "path", path)
	assertNumberField(t, record, "status", http.StatusNoContent)
	assertDurationField(t, record)

	logs := buf.String()
	for _, secret := range []string{
		queryToken,
		queryPass,
		userName,
		userPassword,
		bearer,
		apiKey,
		cookieValue,
		bodyPassword,
		"token=",
		"password=",
		"Bearer ",
		"?",
	} {
		if strings.Contains(logs, secret) {
			t.Errorf("log contains %q: %s", secret, logs)
		}
	}
}

func captureLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

func logRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	line := strings.TrimSpace(buf.String())
	if line == "" || strings.Contains(line, "\n") {
		t.Fatalf("expected one log record, got %q", buf.String())
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("decode log %q: %v", line, err)
	}
	return record
}

func assertStringField(t *testing.T, record map[string]any, key, want string) {
	t.Helper()
	got, ok := record[key].(string)
	if !ok || got != want {
		t.Fatalf("log %s = %#v, want %q", key, record[key], want)
	}
}

func assertNumberField(t *testing.T, record map[string]any, key string, want int) {
	t.Helper()
	got, ok := record[key].(float64)
	if !ok || got != float64(want) {
		t.Fatalf("log %s = %#v, want %d", key, record[key], want)
	}
}

func assertDurationField(t *testing.T, record map[string]any) {
	t.Helper()
	raw, ok := record["duration"].(string)
	if !ok {
		t.Fatalf("log duration = %#v, want a duration string", record["duration"])
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed < 0 {
		t.Fatalf("log duration = %q", raw)
	}
}
