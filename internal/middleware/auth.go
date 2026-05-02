package middleware

import (
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
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		})
	}
}
