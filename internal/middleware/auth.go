package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/drywaters/permitpal/internal/auth"
	"github.com/drywaters/permitpal/internal/model"
	"github.com/drywaters/permitpal/internal/repository"
)

type driverContextKey struct{}

func DriverFromContext(ctx context.Context) (model.Driver, bool) {
	driver, ok := ctx.Value(driverContextKey{}).(model.Driver)
	return driver, ok
}
func WithDriver(ctx context.Context, driver model.Driver) context.Context {
	return context.WithValue(ctx, driverContextKey{}, driver)
}
func RequireAuth(manager *auth.Manager, store repository.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if username, ok := manager.SessionUsername(r); ok {
				driver, err := store.DriverByUsername(r.Context(), username)
				if err == nil {
					next.ServeHTTP(w, r.WithContext(WithDriver(r.Context(), driver)))
					return
				}
				if !errors.Is(err, repository.ErrNotFound) {
					http.Error(w, "Unable to load driver", http.StatusInternalServerError)
					return
				}
			}
			manager.ClearSession(w)
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
