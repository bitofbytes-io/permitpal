package handler

import (
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
		render(w, r, ui.LoginPage("Could not read that login. Try again."))
		return
	}
	if !h.auth.CheckPassword(r.FormValue("password")) {
		render(w, r, ui.LoginPage("That password did not match."))
		return
	}
	h.auth.SetSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.auth.ClearSession(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
