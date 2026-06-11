package handler

import (
	"log/slog"
	"net/http"

	"github.com/drywaters/permitpal/internal/auth"
	"github.com/drywaters/permitpal/internal/ui"
)

type AuthHandler struct {
	auth *auth.Manager
}

func NewAuthHandler(authManager *auth.Manager) *AuthHandler {
	return &AuthHandler{auth: authManager}
}

func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if h.auth.Authenticated(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	render(w, r, ui.LoginPage(""))
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		slog.Info("login failed", "reason", "invalid_request")
		render(w, r, ui.LoginPage("Could not read that login. Try again."))
		return
	}
	if !h.auth.CheckPassword(r.FormValue("password")) {
		slog.Info("login failed", "reason", "invalid_password")
		render(w, r, ui.LoginPage("That password did not match."))
		return
	}
	h.auth.SetSession(w)
	slog.Info("login successful")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.auth.ClearSession(w)
	slog.Info("logout")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
