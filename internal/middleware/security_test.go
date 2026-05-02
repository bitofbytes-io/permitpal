package middleware

import (
	"net/http"
	"net/http/httptest"
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
