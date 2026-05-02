package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/drywaters/permitpal/internal/config"
)

func TestManagerPasswordAndSession(t *testing.T) {
	manager := NewManager(&config.Config{
		Password:        "test-password",
		SessionSecret:   "test-secret",
		SessionCookie:   "permitpal_session",
		DefaultUsername: "driver",
	})

	if !manager.CheckPassword("test-password") {
		t.Fatal("expected password to match")
	}
	if manager.CheckPassword("wrong") {
		t.Fatal("expected wrong password to fail")
	}

	rec := httptest.NewRecorder()
	manager.SetSession(rec)
	req := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range rec.Result().Cookies() {
		req.AddCookie(cookie)
	}
	if !manager.Authenticated(req) {
		t.Fatal("expected generated session to authenticate")
	}
}
