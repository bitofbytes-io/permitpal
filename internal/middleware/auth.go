package middleware

import (
	"log/slog"
	"net/http"

	"github.com/drywaters/permitpal/internal/auth"
)

func RequireAuth(manager *auth.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if manager.Authenticated(r) {
				next.ServeHTTP(w, r)
				return
			}
			slog.Info("authentication required", "path", r.URL.Path)
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		})
	}
}
