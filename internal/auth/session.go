package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/drywaters/permitpal/internal/config"
	"golang.org/x/crypto/bcrypt"
)

type Manager struct {
	cfg         *config.Config
	dummyHashes map[int][]byte
	costs       []int
}

func NewManager(cfg *config.Config) *Manager {
	dummyHashes := make(map[int][]byte)
	for _, hash := range cfg.Users {
		if cost, err := bcrypt.Cost([]byte(hash)); err == nil {
			dummyHashes[cost] = nil
		}
	}
	if len(dummyHashes) == 0 {
		dummyHashes[bcrypt.DefaultCost] = nil
	}
	costs := make([]int, 0, len(dummyHashes))
	for cost := range dummyHashes {
		dummy, err := bcrypt.GenerateFromPassword([]byte("permitpal-unknown-user"), cost)
		if err != nil {
			panic(err)
		}
		dummyHashes[cost] = dummy
		costs = append(costs, cost)
	}
	sort.Ints(costs)
	return &Manager{cfg: cfg, dummyHashes: dummyHashes, costs: costs}
}

func NormalizeUsername(username string) string { return strings.ToLower(strings.TrimSpace(username)) }
func (m *Manager) hasUser(username string) bool {
	_, ok := m.cfg.Users[username]
	return ok
}
func (m *Manager) CheckCredentials(username, password string) bool {
	return m.checkCredentials(username, password, bcrypt.CompareHashAndPassword)
}

func (m *Manager) checkCredentials(username, password string, compare func([]byte, []byte) error) bool {
	username = NormalizeUsername(username)
	hash := m.cfg.Users[username]
	matched := false
	realCost, err := bcrypt.Cost([]byte(hash))
	hasHash := hash != "" && err == nil
	// Compare once at every configured cost, even after a match. Mixed-cost
	// credential files must not expose usernames through different bcrypt work.
	for _, cost := range m.costs {
		candidate := m.dummyHashes[cost]
		isReal := hasHash && cost == realCost
		if isReal {
			candidate = []byte(hash)
		}
		err := compare(candidate, []byte(password))
		if isReal && err == nil {
			matched = true
		}
	}
	return matched
}

func (m *Manager) SetSession(w http.ResponseWriter, username string) {
	expires := time.Now().Add(30 * 24 * time.Hour)
	payload := fmt.Sprintf("%s:%d", NormalizeUsername(username), expires.Unix())
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

func (m *Manager) SessionUsername(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(m.cfg.SessionCookie)
	if err != nil || cookie.Value == "" {
		return "", false
	}

	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return "", false
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	payload := string(payloadBytes)
	if !hmac.Equal([]byte(parts[1]), []byte(m.sign(payload))) {
		return "", false
	}

	payloadParts := strings.Split(payload, ":")
	if len(payloadParts) != 2 {
		return "", false
	}
	expiresUnix, err := strconv.ParseInt(payloadParts[1], 10, 64)
	if err != nil {
		return "", false
	}
	if !m.hasUser(payloadParts[0]) || !time.Now().Before(time.Unix(expiresUnix, 0)) {
		return "", false
	}
	return payloadParts[0], true
}

func (m *Manager) sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(m.cfg.SessionSecret))
	_, _ = mac.Write([]byte(payload))
	username, _, _ := strings.Cut(payload, ":")
	credential := m.cfg.Users[username]
	// Keep the public payload unchanged while revoking cookies after a credential
	// replacement, including removing and later recreating the same username.
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(credential))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
