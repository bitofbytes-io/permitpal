package middleware

import (
	"crypto/rand"
	"net/http"

	"github.com/a-h/templ"
)

// SecurityHeaders sets a Content Security Policy and related headers. Each
// request gets a fresh script nonce in its context for templ.GetNonce, so the
// inline login and Umami loader scripts run while injected scripts do not.
// Styles allow 'unsafe-inline' for the gauges' style attributes and htmx's
// indicator styles. Traefik adds HSTS and the Permissions-Policy in production.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := rand.Text()
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; "+
			"script-src 'self' 'nonce-"+nonce+"'; "+
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
			"font-src 'self' https://fonts.gstatic.com; "+
			"img-src 'self'; "+
			"connect-src 'self'; "+
			"object-src 'none'; "+
			"base-uri 'none'; "+
			"form-action 'self'; "+
			"frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r.WithContext(templ.WithNonce(r.Context(), nonce)))
	})
}
