package handler

import (
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/drywaters/permitpal/internal/auth"
	"github.com/drywaters/permitpal/internal/config"
	"github.com/drywaters/permitpal/internal/middleware"
	"github.com/drywaters/permitpal/internal/repository"
	"github.com/drywaters/permitpal/internal/ui"
)

type AuthHandler struct {
	auth    *auth.Manager
	store   repository.Store
	limiter *auth.LoginLimiter
}

func NewAuthHandler(authManager *auth.Manager, store repository.Store, limiter *auth.LoginLimiter) *AuthHandler {
	return &AuthHandler{auth: authManager, store: store, limiter: limiter}
}

func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if username, ok := h.auth.SessionUsername(r); ok {
		if _, err := h.store.DriverByUsername(r.Context(), username); err == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	render(w, r, ui.LoginPage("", ""))
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		slog.Info("login failed", "reason", "invalid_request")
		render(w, r, ui.LoginPage("Could not read that login. Try again.", ""))
		return
	}
	username := auth.NormalizeUsername(r.FormValue("username"))
	clientIP, ipVerified := middleware.VerifiedClientIP(r)
	limitIP := ""
	if ipVerified {
		limitIP = clientIP
	}
	// Usernames that cannot exist get no bucket of their own, so callers
	// cannot grow the limiter with arbitrary keys; the IP limit still applies.
	validUsername := config.ValidUsername(username)
	limitUsername := ""
	if validUsername {
		limitUsername = username
	}
	logAttrs := []any{"username", truncateRunes(username, maxLoggedUsername), "username_length", len(username), "client_ip", clientIP, "client_ip_verified", ipVerified}
	if wait, ok := h.limiter.Allow(limitIP, limitUsername); !ok {
		slog.Warn("login failed", append([]any{"reason", "rate_limited"}, logAttrs...)...)
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		renderStatus(w, r, http.StatusTooManyRequests, ui.LoginPage("Too many failed login attempts. Try again later.", r.FormValue("username")))
		return
	}
	if !validUsername {
		slog.Info("login failed", append([]any{"reason", "invalid_username"}, logAttrs...)...)
		render(w, r, ui.LoginPage("That username and password did not match.", r.FormValue("username")))
		return
	}
	if !h.auth.CheckCredentials(username, r.FormValue("password")) {
		slog.Info("login failed", append([]any{"reason", "invalid_password"}, logAttrs...)...)
		render(w, r, ui.LoginPage("That username and password did not match.", r.FormValue("username")))
		return
	}
	h.limiter.Succeed(limitIP, username)
	if _, err := h.store.EnsureDriver(r.Context(), username, time.Now()); err != nil {
		http.Error(w, "Unable to load driver", http.StatusInternalServerError)
		return
	}
	h.auth.SetSession(w, username)
	slog.Info("login successful", "username", username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// maxLoggedUsername bounds attacker-controlled usernames in failure logs.
const maxLoggedUsername = 32

func truncateRunes(value string, limit int) string {
	for i := range value {
		if limit == 0 {
			return value[:i]
		}
		limit--
	}
	return value
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.auth.ClearSession(w)
	slog.Info("logout")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
