package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/drywaters/permitpal/internal/auth"
	"github.com/drywaters/permitpal/internal/repository"
	"github.com/drywaters/permitpal/internal/ui"
)

type AuthHandler struct {
	auth  *auth.Manager
	store repository.Store
}

func NewAuthHandler(authManager *auth.Manager, store repository.Store) *AuthHandler {
	return &AuthHandler{auth: authManager, store: store}
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
	if !h.auth.CheckCredentials(username, r.FormValue("password")) {
		slog.Info("login failed", "reason", "invalid_password")
		render(w, r, ui.LoginPage("That username and password did not match.", r.FormValue("username")))
		return
	}
	if _, err := h.store.EnsureDriver(r.Context(), username, time.Now()); err != nil {
		http.Error(w, "Unable to load driver", http.StatusInternalServerError)
		return
	}
	h.auth.SetSession(w, username)
	slog.Info("login successful", "username", username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.auth.ClearSession(w)
	slog.Info("logout")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
