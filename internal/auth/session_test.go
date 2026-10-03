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
	manager.SetSession(rec, "aiden", 0)
	cookie := rec.Result().Cookies()[0]
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	if session, ok := manager.Session(req); !ok || session != (Session{Username: "aiden"}) {
		t.Fatalf("session=%+v,%v", session, ok)
	}
	parts := strings.Split(cookie.Value, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[0])
	parts[0] = base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(payload), "aiden", "caleb", 1)))
	tampered := httptest.NewRequest("GET", "/", nil)
	tampered.AddCookie(&http.Cookie{Name: cookie.Name, Value: strings.Join(parts, ".")})
	if _, ok := manager.Session(tampered); ok {
		t.Fatal("tampered username accepted")
	}
	expiredPayload := fmt.Sprintf("aiden:%d", time.Now().Add(-time.Hour).Unix())
	expired := httptest.NewRequest("GET", "/", nil)
	expired.AddCookie(&http.Cookie{Name: cookie.Name, Value: base64.RawURLEncoding.EncodeToString([]byte(expiredPayload)) + "." + manager.sign(expiredPayload)})
	if _, ok := manager.Session(expired); ok {
		t.Fatal("expired session accepted")
	}
	delete(cfg.Users, "aiden")
	if _, ok := manager.Session(req); ok {
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
		manager.SetSession(rec, username, 0)
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(rec.Result().Cookies()[0])
		return req
	}
	oldRequest := requestWithCookie("aiden")
	if _, ok := manager.Session(oldRequest); !ok {
		t.Fatal("initial cookie rejected")
	}
	cfg.Users = map[string]string{"aiden": string(nextHash)}
	if _, ok := manager.Session(oldRequest); ok {
		t.Fatal("old cookie accepted after credential replacement")
	}
	if _, ok := manager.Session(requestWithCookie("aiden")); !ok {
		t.Fatal("new cookie rejected")
	}
	if _, ok := manager.Session(requestWithCookie("unknown")); ok {
		t.Fatal("unknown user cookie accepted")
	}
}

func TestSessionGenerations(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(&config.Config{Users: map[string]string{"aiden": string(hash)}, SessionCookie: "session", SessionSecret: "a-session-secret-that-is-at-least-32-characters"})
	expires := time.Now().Add(time.Hour).Unix()
	signed := func(payload string) *http.Request {
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + manager.sign(payload)})
		return req
	}
	issued := func(generation int64) (string, *http.Request) {
		rec := httptest.NewRecorder()
		manager.SetSession(rec, "aiden", generation)
		cookie := rec.Result().Cookies()[0]
		payload, err := base64.RawURLEncoding.DecodeString(strings.Split(cookie.Value, ".")[0])
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(cookie)
		return string(payload), req
	}

	// Cookies from before generations are username:expires and count as generation 0.
	if session, ok := manager.Session(signed(fmt.Sprintf("aiden:%d", expires))); !ok || session != (Session{Username: "aiden"}) {
		t.Fatalf("old-format cookie: session=%+v ok=%v", session, ok)
	}
	// Generation 0 is still issued in that format, so the previous release accepts it.
	if payload, req := issued(0); strings.Count(payload, ":") != 1 {
		t.Fatalf("generation 0 payload %q is not username:expires", payload)
	} else if session, ok := manager.Session(req); !ok || session.Generation != 0 {
		t.Fatalf("generation 0 cookie: session=%+v ok=%v", session, ok)
	}
	payload, req := issued(7)
	if session, ok := manager.Session(req); !ok || session != (Session{Username: "aiden", Generation: 7}) || !strings.HasSuffix(payload, ":7") {
		t.Fatalf("generation 7 cookie %q: session=%+v ok=%v", payload, session, ok)
	}
	// Raising the generation without re-signing is rejected.
	forged := req.Clone(req.Context())
	cookie, _ := req.Cookie("session")
	forged.Header.Del("Cookie")
	forged.AddCookie(&http.Cookie{Name: "session", Value: base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSuffix(payload, "7")+"8")) + "." + strings.Split(cookie.Value, ".")[1]})
	if _, ok := manager.Session(forged); ok {
		t.Fatal("re-numbered generation accepted")
	}
	for _, suffix := range []string{":-1", ":x", ":", ":1:2"} {
		if _, ok := manager.Session(signed(fmt.Sprintf("aiden:%d%s", expires, suffix))); ok {
			t.Fatalf("malformed generation %q accepted", suffix)
		}
	}
}
