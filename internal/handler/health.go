package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// readyTimeout bounds the database ping so a hung connection fails the check
// well before the orchestrator's healthcheck times out.
const readyTimeout = 2 * time.Second

// Pinger reports whether a dependency is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

type readiness struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

// Health reports that the process is up. It checks no dependencies.
func Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Ready reports whether the database answers a ping. It responds 503 when it
// does not, and logs the cause instead of returning it.
func Ready(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
		defer cancel()

		status, body := http.StatusOK, readiness{Status: "ok", DB: "ok"}
		if err := db.Ping(ctx); err != nil {
			slog.Error("readiness check failed", "error", err, "request_id", chimw.GetReqID(r.Context()))
			status, body = http.StatusServiceUnavailable, readiness{Status: "degraded", DB: "error"}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
}
