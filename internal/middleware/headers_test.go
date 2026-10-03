package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestSecurityHeadersSetAFreshScriptNonce(t *testing.T) {
	var nonces []string
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonces = append(nonces, templ.GetNonce(r.Context()))
	}))
	for range 2 {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		nonce := nonces[len(nonces)-1]
		csp := rec.Header().Get("Content-Security-Policy")
		if nonce == "" || !strings.Contains(csp, "script-src 'self' 'nonce-"+nonce+"';") {
			t.Fatalf("nonce %q not in CSP %q", nonce, csp)
		}
		for _, want := range []string{"default-src 'self'", "object-src 'none'", "frame-ancestors 'none'", "form-action 'self'"} {
			if !strings.Contains(csp, want) {
				t.Fatalf("CSP %q is missing %s", csp, want)
			}
		}
		if scriptSrc, _, _ := strings.Cut(csp[strings.Index(csp, "script-src"):], ";"); strings.Contains(scriptSrc, "unsafe") {
			t.Fatalf("script-src must not allow unsafe sources: %q", scriptSrc)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("X-Frame-Options") != "DENY" || rec.Header().Get("Referrer-Policy") == "" {
			t.Fatalf("missing security headers: %v", rec.Header())
		}
	}
	if nonces[0] == nonces[1] {
		t.Fatal("nonce was reused across requests")
	}
}
