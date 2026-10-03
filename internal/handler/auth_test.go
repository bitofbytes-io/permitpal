package handler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/auth"
	"github.com/drywaters/permitpal/internal/config"
	"github.com/drywaters/permitpal/internal/middleware"
	"github.com/drywaters/permitpal/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

func TestLoginRejectsInvalidUsernamesWithoutUsernameBuckets(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Users: map[string]string{"aiden": string(hash)}, SessionSecret: "test-session-secret-32-chars-ok", SessionCookie: "permitpal_session"}
	const maxFailures = 3
	limiter := auth.NewLoginLimiter(maxFailures, 15*time.Minute)
	login := middleware.RealIP([]netip.Prefix{netip.MustParsePrefix("10.0.1.0/24")})(http.HandlerFunc(
		NewAuthHandler(auth.NewManager(cfg), repository.NewMemoryStore(time.Now()), limiter).Login))
	post := func(username string) *httptest.ResponseRecorder {
		t.Helper()
		form := url.Values{"username": {username}, "password": {"wrong"}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "10.0.1.5:4000"
		req.Header.Set("X-Forwarded-For", "198.51.100.10")
		rec := httptest.NewRecorder()
		login.ServeHTTP(rec, req)
		return rec
	}

	long := strings.Repeat("x", 8000)
	for _, username := range []string{long, "bad name!", strings.Repeat("a", 33)} {
		rec := post(username)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "did not match") {
			t.Fatalf("status=%d for invalid username", rec.Code)
		}
	}
	if !strings.Contains(logs.String(), "reason=invalid_username") || strings.Count(logs.String(), "reason=invalid_password") != 0 {
		t.Fatalf("invalid usernames reached the password check: %s", logs.String())
	}
	if strings.Contains(logs.String(), strings.Repeat("x", 33)) || !strings.Contains(logs.String(), "username="+strings.Repeat("x", 32)+" username_length=8000") {
		t.Fatalf("long username was not truncated in logs: %.300s", logs.String())
	}
	// The three invalid attempts used only the verified client IP's bucket.
	if rec := post("aiden"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d, want 429 once invalid usernames exhaust the IP limit", rec.Code)
	}
}

type failingLogoutStore struct{ repository.Store }

func (failingLogoutStore) EndSessions(context.Context, string, int64) error {
	return errors.New("database unavailable")
}

func TestLogoutKeepsTheCookieWhenSessionsCannotEnd(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	manager := auth.NewManager(&config.Config{Users: map[string]string{"aiden": string(hash)}, SessionSecret: "test-session-secret-32-chars-ok", SessionCookie: "permitpal_session"})
	session := httptest.NewRecorder()
	manager.SetSession(session, "aiden", 0)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(session.Result().Cookies()[0])
	rec := httptest.NewRecorder()
	NewAuthHandler(manager, failingLogoutStore{}, auth.NewLoginLimiter(3, time.Minute)).Logout(rec, req)
	if rec.Code != http.StatusInternalServerError || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("status=%d cookies=%v; want 500 and the cookie kept for a retry", rec.Code, rec.Result().Cookies())
	}
}
