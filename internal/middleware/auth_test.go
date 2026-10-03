package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/auth"
	"github.com/drywaters/permitpal/internal/config"
	"github.com/drywaters/permitpal/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

func TestRequireAuthLoadsDriverAndRejectsMissingDriver(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("local-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	manager := auth.NewManager(&config.Config{Users: map[string]string{"aiden": string(hash)}, SessionCookie: "session", SessionSecret: "long-enough-test-secret-32-characters"})
	store := repository.NewMemoryStore(time.Now())
	session := httptest.NewRecorder()
	manager.SetSession(session, "aiden", 0)
	cookie := session.Result().Cookies()[0]
	called := false
	handler := RequireAuth(manager, store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		driver, ok := DriverFromContext(r.Context())
		if !ok || driver.Username != "aiden" {
			t.Fatal("missing driver context")
		}
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	for _, htmx := range []bool{false, true} {
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(cookie)
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if called {
			t.Fatal("missing driver was authorized")
		}
		if htmx {
			if rec.Code != 204 || rec.Header().Get("HX-Redirect") != "/login" {
				t.Fatal("missing HTMX redirect")
			}
		} else if rec.Code != 303 || rec.Header().Get("Location") != "/login" {
			t.Fatal("missing login redirect")
		}
		if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
			t.Fatal("cookie not cleared")
		}
	}
	if _, err := store.EnsureDriver(context.Background(), "aiden", time.Now()); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !called || rec.Code != 200 {
		t.Fatal("existing driver not authorized")
	}
}

// legacyCookie builds a session cookie exactly as releases before session
// generations did: base64(username:expires) "." HMAC-SHA256 over the payload,
// a zero byte and the user's bcrypt hash.
func legacyCookie(name, secret, hash, username string, expires time.Time) *http.Cookie {
	payload := fmt.Sprintf("%s:%d", username, expires.Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	mac.Write([]byte{0})
	mac.Write([]byte(hash))
	return &http.Cookie{Name: name, Value: base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))}
}

func TestOldFormatCookieAuthenticatesUntilLogout(t *testing.T) {
	ctx := context.Background()
	hash, err := bcrypt.GenerateFromPassword([]byte("local-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	const secret = "long-enough-test-secret-32-characters"
	manager := auth.NewManager(&config.Config{Users: map[string]string{"aiden": string(hash)}, SessionCookie: "session", SessionSecret: secret})
	store := repository.NewMemoryStore(time.Now())
	if _, err := store.EnsureDriver(ctx, "aiden", time.Now()); err != nil {
		t.Fatal(err)
	}
	handler := RequireAuth(manager, store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	allowed := func(cookie *http.Cookie) bool {
		t.Helper()
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code == http.StatusOK
	}

	old := legacyCookie("session", secret, string(hash), "aiden", time.Now().Add(24*time.Hour))
	if !allowed(old) {
		t.Fatal("a cookie issued before session generations was rejected")
	}
	if err := store.EndSessions(ctx, "aiden", 0); err != nil {
		t.Fatal(err)
	}
	if allowed(old) {
		t.Fatal("old cookie still accepted after logout")
	}
	rec := httptest.NewRecorder()
	manager.SetSession(rec, "aiden", 1)
	if fresh := rec.Result().Cookies()[0]; !allowed(fresh) {
		t.Fatal("cookie from the current generation rejected")
	}
	// A logout replayed with the ended cookie must not end the newer session.
	if err := store.EndSessions(ctx, "aiden", 0); err != nil {
		t.Fatal(err)
	}
	if driver, _ := store.DriverByUsername(ctx, "aiden"); driver.SessionGeneration != 1 {
		t.Fatalf("stale logout moved the generation to %d", driver.SessionGeneration)
	}
}
