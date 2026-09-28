package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/config"
	"github.com/drywaters/permitpal/internal/model"
	"github.com/drywaters/permitpal/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

func TestUnauthenticatedUserSeesLogin(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	res := doGet(t, http.DefaultClient, ts.URL+"/login")
	body := readBody(t, res)
	if !strings.Contains(body, "PermitPal") || !strings.Contains(body, "Sign in") {
		t.Fatalf("login page did not render expected copy: %s", body)
	}
	// The splash lives on the root so Safari's keyboard and overscroll areas never show the page color.
	if !strings.Contains(body, `<html lang="en" class="login-page">`) {
		t.Fatalf("login page root is missing the login-page class: %s", body)
	}
	username := strings.Index(body, `id="username"`)
	password := strings.Index(body, `id="password"`)
	if username < 0 || password < 0 || username > password {
		t.Fatalf("login form must render username before password: %s", body)
	}
	// Password managers insert a focusable button in the username field; Tab must skip it.
	for _, want := range []string{
		`document.getElementById("username").addEventListener("keydown"`,
		`event.key === "Tab" && !event.shiftKey`,
		`document.getElementById("password").focus()`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("login page is missing the username Tab handler %q: %s", want, body)
		}
	}
}

func TestAuthenticatedDashboardAndHTMXUpdates(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	client := ts.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	loginForm := url.Values{"username": {"aiden"}, "password": {"test-password"}}
	res := doPostForm(t, client, ts.URL+"/login", loginForm)
	closeBody(t, res)

	res = doGet(t, client, ts.URL+"/")
	body := readBody(t, res)
	if !strings.Contains(body, "Road Test Skill Checklist") || !strings.Contains(body, "Use of lane") {
		t.Fatalf("dashboard missing checklist content: %s", body)
	}
	assertDecimalHourInput(t, body, "total_hours", `(60(\.0)?|[0-5]?[0-9](\.[0-9])?|\.[0-9])`)
	assertDecimalHourInput(t, body, "night_hours", `(10(\.0)?|[0-9](\.[0-9])?|\.[0-9])`)

	update := url.Values{
		"rating":   {"good"},
		"rated_on": {"2026-05-02"},
		"notes":    {"Clean mirror checks."},
	}
	res = doPostForm(t, client, ts.URL+"/requirements/use-of-lane", update)
	row := readBody(t, res)
	if !strings.Contains(row, "Good") || !strings.Contains(row, "Clean mirror checks.") {
		t.Fatalf("requirement partial missing updated content: %s", row)
	}

	progress := url.Values{
		"total_hours":        {"42.5"},
		"night_hours":        {"8.5"},
		"permit_issue_date":  {"2025-10-18"},
		"unexpected_ignored": {"x"},
	}
	res = doPostForm(t, client, ts.URL+"/profile", progress)
	panel := readBody(t, res)
	if !strings.Contains(panel, "42.5 total hours") || !strings.Contains(panel, "8.5 night hours") {
		t.Fatalf("progress partial missing updated values: %s", panel)
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(newTestApp(t).Router())
}

func newTestApp(t *testing.T) *Server {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Users:         map[string]string{"aiden": string(hash), "caleb": string(hash)},
		AppEnv:        "development",
		DataStore:     config.DataStoreMemory,
		Port:          "4600",
		SessionSecret: "test-session-secret-32-chars-ok",
		SessionCookie: "permitpal_session",
		Location:      time.Local,
	}
	store := repository.NewMemoryStore(time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local))
	caleb, err := store.EnsureDriver(context.Background(), "caleb", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProfile(context.Background(), caleb.ID, model.Profile{TotalHours: 56, NightHours: 8}); err != nil {
		t.Fatal(err)
	}
	return New(cfg, store, slog.Default())
}

func doGet(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func doPostForm(t *testing.T, client *http.Client, target string, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", u.Scheme+"://"+u.Host)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	closeBody(t, res)
	return string(data)
}

func closeBody(t *testing.T, res *http.Response) {
	t.Helper()
	if err := res.Body.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertDecimalHourInput(t *testing.T, body, id, pattern string) {
	t.Helper()
	tag := inputTagByID(t, body, id)
	required := []string{
		`name="` + id + `"`,
		`type="text"`,
		`inputmode="decimal"`,
		`pattern="` + pattern + `"`,
		`title="Enter 0 to`,
	}
	for _, attr := range required {
		if !strings.Contains(tag, attr) {
			t.Fatalf("%s input missing %s: %s", id, attr, tag)
		}
	}
	if strings.Contains(tag, `oninput="this.value`) {
		t.Fatalf("%s input still mutates value on input: %s", id, tag)
	}
}

func inputTagByID(t *testing.T, body, id string) string {
	t.Helper()
	idAttr := `id="` + id + `"`
	idIndex := strings.Index(body, idAttr)
	if idIndex < 0 {
		t.Fatalf("input %s not found in body: %s", id, body)
	}
	start := strings.LastIndex(body[:idIndex], "<input")
	end := strings.Index(body[idIndex:], ">")
	if start < 0 || end < 0 {
		t.Fatalf("input %s tag could not be extracted from body: %s", id, body)
	}
	return body[start : idIndex+end+1]
}

func TestSeparateLoginsAndInvalidCredentials(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	client := ts.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	for _, username := range []string{"aiden", "missing"} {
		body := readBody(t, doPostForm(t, client, ts.URL+"/login", url.Values{"username": {username}, "password": {"wrong"}}))
		if !strings.Contains(body, "That username and password did not match.") || !strings.Contains(body, `value="`+username+`"`) {
			t.Fatal("invalid credentials should retain username and show neutral error")
		}
	}
	body := readBody(t, doPostForm(t, client, ts.URL+"/login", url.Values{"username": {" AIDEN "}, "password": {"test-password"}}))
	if !strings.Contains(body, "Welcome back, Aiden!") || !strings.Contains(body, "0.0 total hours") || strings.Count(body, `class="row-number"`) != 17 {
		t.Fatal("Aiden should have a fresh 17-skill tracker")
	}
	closeBody(t, doPostForm(t, client, ts.URL+"/profile", url.Values{"total_hours": {"20"}, "night_hours": {"2"}}))
	closeBody(t, doPostForm(t, client, ts.URL+"/logout", nil))
	body = readBody(t, doPostForm(t, client, ts.URL+"/login", url.Values{"username": {"caleb"}, "password": {"test-password"}}))
	if !strings.Contains(body, "Welcome back, Caleb!") || !strings.Contains(body, "56.0 total hours") || !strings.Contains(body, "8.0 night hours") {
		t.Fatal("Caleb lost existing progress")
	}
}

func TestLoginRateLimits(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("10.0.1.0/24")}
	newLogin := func(t *testing.T, trusted []netip.Prefix) func(remoteAddr, forwardedFor, username, password string) *httptest.ResponseRecorder {
		app := newTestApp(t)
		app.cfg.TrustedProxies = trusted
		router := app.Router()
		return func(remoteAddr, forwardedFor, username, password string) *httptest.ResponseRecorder {
			t.Helper()
			form := url.Values{"username": {username}, "password": {password}}
			req := httptest.NewRequest(http.MethodPost, "http://permitpal.test/login", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", "http://permitpal.test")
			req.RemoteAddr = remoteAddr
			if forwardedFor != "" {
				req.Header.Set("X-Forwarded-For", forwardedFor)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			return rec
		}
	}
	// failFromOneIP spends the per-IP budget using a different username each time.
	failFromOneIP := func(t *testing.T, login func(string, string, string, string) *httptest.ResponseRecorder, remoteAddr string, forwardedFor func(int) string) {
		t.Helper()
		for i := range maxLoginFailures {
			rec := login(remoteAddr, forwardedFor(i), fmt.Sprintf("guess%d", i), "wrong")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "did not match") {
				t.Fatalf("failure %d: status=%d", i+1, rec.Code)
			}
		}
	}
	assertBlocked := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "900" || !strings.Contains(rec.Body.String(), "Too many failed login attempts") {
			t.Fatalf("status=%d retry-after=%q, want 429 with Retry-After 900", rec.Code, rec.Header().Get("Retry-After"))
		}
	}
	assertLoggedIn := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status=%d, want 303", rec.Code)
		}
	}

	t.Run("no trusted proxies skips the IP limit", func(t *testing.T) {
		login := newLogin(t, nil)
		failFromOneIP(t, login, "203.0.113.7:4000", func(int) string { return "" })
		assertLoggedIn(t, login("203.0.113.7:4000", "", "aiden", "test-password"))
	})

	t.Run("username limit applies without trusted proxies", func(t *testing.T) {
		login := newLogin(t, nil)
		for i := range maxLoginFailures {
			login(fmt.Sprintf("203.0.113.%d:4000", i+1), "", "caleb", "wrong")
		}
		assertBlocked(t, login("203.0.113.99:4000", "", "caleb", "test-password"))
		assertLoggedIn(t, login("203.0.113.99:4000", "", "aiden", "test-password"))
	})

	t.Run("trusted forwarded client is limited", func(t *testing.T) {
		login := newLogin(t, proxies)
		failFromOneIP(t, login, "10.0.1.5:4000", func(int) string { return "198.51.100.10" })
		assertBlocked(t, login("10.0.1.6:4000", "198.51.100.10", "aiden", "test-password"))
		assertLoggedIn(t, login("10.0.1.5:4000", "198.51.100.11", "aiden", "test-password"))
	})

	t.Run("proxy peer without forwarded header skips the IP limit", func(t *testing.T) {
		login := newLogin(t, proxies)
		failFromOneIP(t, login, "10.0.1.5:4000", func(int) string { return "" })
		assertLoggedIn(t, login("10.0.1.5:4000", "", "aiden", "test-password"))
	})

	t.Run("spoofed header from untrusted peer keys on the peer", func(t *testing.T) {
		login := newLogin(t, proxies)
		failFromOneIP(t, login, "203.0.113.7:4000", func(i int) string { return fmt.Sprintf("198.51.100.%d", i) })
		assertBlocked(t, login("203.0.113.7:4000", "198.51.100.200", "aiden", "test-password"))
	})
}
