package middleware

import (
	"context"
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
	manager.SetSession(session, "aiden")
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
