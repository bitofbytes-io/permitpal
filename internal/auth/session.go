package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/drywaters/permitpal/internal/config"
	"golang.org/x/crypto/bcrypt"
)

type Manager struct {
	cfg *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg}
}

func (m *Manager) CheckPassword(password string) bool {
	if m.cfg.PasswordHash != "" {
		return bcrypt.CompareHashAndPassword([]byte(m.cfg.PasswordHash), []byte(password)) == nil
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(m.cfg.Password)) == 1
}

func (m *Manager) SetSession(w http.ResponseWriter) {
	expires := time.Now().Add(30 * 24 * time.Hour)
	payload := fmt.Sprintf("%s:%d", m.cfg.DefaultUsername, expires.Unix())
	signature := m.sign(payload)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + signature

	http.SetCookie(w, &http.Cookie{
		Name:     m.cfg.SessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.cfg.SecureCookies,
	})
}

func (m *Manager) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cfg.SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.cfg.SecureCookies,
	})
}

func (m *Manager) Authenticated(r *http.Request) bool {
	cookie, err := r.Cookie(m.cfg.SessionCookie)
	if err != nil || cookie.Value == "" {
		return false
	}

	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return false
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	payload := string(payloadBytes)
	if !hmac.Equal([]byte(parts[1]), []byte(m.sign(payload))) {
		return false
	}

	payloadParts := strings.Split(payload, ":")
	if len(payloadParts) != 2 {
		return false
	}
	expiresUnix, err := strconv.ParseInt(payloadParts[1], 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Before(time.Unix(expiresUnix, 0))
}

func (m *Manager) sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(m.cfg.SessionSecret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
