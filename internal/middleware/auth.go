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

// SessionDriver returns the driver signed in to r. ok is false when the
// cookie is missing or invalid, names no driver, or carries a generation
// that a logout has ended.
func SessionDriver(r *http.Request, manager *auth.Manager, store repository.Store) (driver model.Driver, ok bool, err error) {
	session, ok := manager.Session(r)
	if !ok {
		return model.Driver{}, false, nil
	}
	driver, err = store.DriverByUsername(r.Context(), session.Username)
	if errors.Is(err, repository.ErrNotFound) {
		return model.Driver{}, false, nil
	}
	if err != nil {
		return model.Driver{}, false, err
	}
	if driver.SessionGeneration != session.Generation {
		return model.Driver{}, false, nil
	}
	return driver, true, nil
}

func RequireAuth(manager *auth.Manager, store repository.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			driver, ok, err := SessionDriver(r, manager, store)
			if err != nil {
				ServerError(w, r, "Unable to load driver", err)
				return
			}
			if ok {
				next.ServeHTTP(w, r.WithContext(WithDriver(r.Context(), driver)))
				return
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
