package auth

import (
	"encoding/base64"
	"fmt"
	"github.com/drywaters/permitpal/internal/config"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestManagerCredentialsAndSession(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Users: map[string]string{"aiden": string(hash)}, SessionSecret: "test-session-secret-32-chars-ok", SessionCookie: "permitpal_session"}
	manager := NewManager(cfg)
	if !manager.CheckCredentials(" AIDEN ", "test-password") {
		t.Fatal("normalized login failed")
	}
	for _, username := range []string{"aiden", "missing"} {
		if manager.CheckCredentials(username, "wrong") {
			t.Fatal("wrong password passed")
		}
	}
	if manager.CheckCredentials("missing", "test-password") {
		t.Fatal("unknown user passed")
	}
	rec := httptest.NewRecorder()
	manager.SetSession(rec, "aiden")
	cookie := rec.Result().Cookies()[0]
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	if user, ok := manager.SessionUsername(req); !ok || user != "aiden" {
		t.Fatalf("session=%q,%v", user, ok)
	}
	parts := strings.Split(cookie.Value, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[0])
	parts[0] = base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(payload), "aiden", "caleb", 1)))
	tampered := httptest.NewRequest("GET", "/", nil)
	tampered.AddCookie(&http.Cookie{Name: cookie.Name, Value: strings.Join(parts, ".")})
	if _, ok := manager.SessionUsername(tampered); ok {
		t.Fatal("tampered username accepted")
	}
	expiredPayload := fmt.Sprintf("aiden:%d", time.Now().Add(-time.Hour).Unix())
	expired := httptest.NewRequest("GET", "/", nil)
	expired.AddCookie(&http.Cookie{Name: cookie.Name, Value: base64.RawURLEncoding.EncodeToString([]byte(expiredPayload)) + "." + manager.sign(expiredPayload)})
	if _, ok := manager.SessionUsername(expired); ok {
		t.Fatal("expired session accepted")
	}
	delete(cfg.Users, "aiden")
	if _, ok := manager.SessionUsername(req); ok {
		t.Fatal("removed user accepted")
	}
}

func TestMixedCostCredentialsUseSameWorkload(t *testing.T) {
	low, err := bcrypt.GenerateFromPassword([]byte("low-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	high, err := bcrypt.GenerateFromPassword([]byte("high-password"), bcrypt.MinCost+1)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Users: map[string]string{"aiden": string(low), "same-cost": string(low), "caleb": string(high)}}
	manager := NewManager(cfg)
	for _, test := range []struct {
		username, password string
		want               bool
		realHash           string
	}{
		{"aiden", "low-password", true, string(low)},
		{" AIDEN ", "wrong", false, string(low)},
		{"same-cost", "low-password", true, string(low)},
		{"caleb", "high-password", true, string(high)},
		{"caleb", "wrong", false, string(high)},
		{"missing", "low-password", false, ""},
		{"missing", "permitpal-unknown-user", false, ""},
	} {
		t.Run(test.username+"/"+test.password, func(t *testing.T) {
			var costs []int
			realComparisons := 0
			compare := func(hash, password []byte) error {
				cost, err := bcrypt.Cost(hash)
				if err != nil {
					t.Fatal(err)
				}
				costs = append(costs, cost)
				if string(hash) == test.realHash {
					realComparisons++
				}
				return bcrypt.CompareHashAndPassword(hash, password)
			}
			if got := manager.checkCredentials(test.username, test.password, compare); got != test.want {
				t.Fatalf("matched=%v, want %v", got, test.want)
			}
			if len(costs) != 2 || costs[0] != bcrypt.MinCost || costs[1] != bcrypt.MinCost+1 {
				t.Fatalf("bcrypt comparisons=%v; want one each at configured costs", costs)
			}
			wantReal := 0
			if test.realHash != "" {
				wantReal = 1
			}
			if realComparisons != wantReal {
				t.Fatalf("real credential comparisons=%d, want %d", realComparisons, wantReal)
			}
		})
	}
}

func TestCredentialReplacementRevokesExistingSessions(t *testing.T) {
	firstHash, err := bcrypt.GenerateFromPassword([]byte("first-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	nextHash, err := bcrypt.GenerateFromPassword([]byte("next-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Users: map[string]string{"aiden": string(firstHash)}, SessionCookie: "session", SessionSecret: "a-session-secret-that-is-at-least-32-characters"}
	manager := NewManager(cfg)
	requestWithCookie := func(username string) *http.Request {
		rec := httptest.NewRecorder()
		manager.SetSession(rec, username)
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(rec.Result().Cookies()[0])
		return req
	}
	oldRequest := requestWithCookie("aiden")
	if _, ok := manager.SessionUsername(oldRequest); !ok {
		t.Fatal("initial cookie rejected")
	}
	cfg.Users = map[string]string{"aiden": string(nextHash)}
	if _, ok := manager.SessionUsername(oldRequest); ok {
		t.Fatal("old cookie accepted after credential replacement")
	}
	if _, ok := manager.SessionUsername(requestWithCookie("aiden")); !ok {
		t.Fatal("new cookie rejected")
	}
	if _, ok := manager.SessionUsername(requestWithCookie("unknown")); ok {
		t.Fatal("unknown user cookie accepted")
	}
}
