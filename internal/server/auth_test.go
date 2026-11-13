package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
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

func TestRouterRejectsCrossOriginPosts(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	client := ts.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	closeBody(t, doPostForm(t, client, ts.URL+"/login", url.Values{"username": {"caleb"}, "password": {"test-password"}}))

	forms := map[string]url.Values{
		"/login":                    {"username": {"aiden"}, "password": {"test-password"}},
		"/logout":                   {},
		"/profile":                  {"total_hours": {"1"}, "night_hours": {"1"}},
		"/requirements/use-of-lane": {"rating": {"bad"}, "notes": {"cross-site"}},
	}
	for name, setOrigin := range map[string]func(*http.Request){
		"foreign origin":       func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") },
		"foreign referer":      func(r *http.Request) { r.Header.Set("Referer", "https://attacker.example/form") },
		"no origin or referer": func(*http.Request) {},
	} {
		for path, form := range forms {
			t.Run(name+path, func(t *testing.T) {
				req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+path, strings.NewReader(form.Encode()))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				setOrigin(req)
				res, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				if body := readBody(t, res); res.StatusCode != http.StatusForbidden || !strings.Contains(body, "Invalid cross-site request") {
					t.Fatalf("status=%d body=%q, want 403", res.StatusCode, body)
				}
			})
		}
	}

	// None of the rejected posts logged Caleb out, switched accounts or changed his tracker.
	body := readBody(t, doGet(t, client, ts.URL+"/"))
	if !strings.Contains(body, "Welcome back, Caleb!") || !strings.Contains(body, "56.0 total hours") || strings.Contains(body, "cross-site") {
		t.Fatalf("a cross-origin post changed state: %s", body)
	}
}

func TestLogoutEndsSessionsOnEveryDevice(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	signIn := func() *http.Client {
		t.Helper()
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatal(err)
		}
		client := &http.Client{Jar: jar}
		closeBody(t, doPostForm(t, client, ts.URL+"/login", url.Values{"username": {"caleb"}, "password": {"test-password"}}))
		return client
	}
	signedIn := func(client *http.Client) bool {
		t.Helper()
		return strings.Contains(readBody(t, doGet(t, client, ts.URL+"/")), "Welcome back, Caleb!")
	}
	phone, laptop := signIn(), signIn()
	// Keep the laptop's cookie: the browser drops it once the server clears it.
	laptopURL, _ := url.Parse(ts.URL)
	laptopCookies := laptop.Jar.Cookies(laptopURL)
	if !signedIn(phone) || !signedIn(laptop) {
		t.Fatal("both devices should start signed in")
	}
	closeBody(t, doPostForm(t, phone, ts.URL+"/logout", nil))
	if signedIn(phone) || signedIn(laptop) {
		t.Fatal("logout left a device signed in")
	}
	again := signIn()
	if !signedIn(again) {
		t.Fatal("signing in after logout failed")
	}
	replay, _ := cookiejar.New(nil)
	replay.SetCookies(laptopURL, laptopCookies)
	if signedIn(&http.Client{Jar: replay}) {
		t.Fatal("a cookie from before the logout still works")
	}
	if !signedIn(again) {
		t.Fatal("the replayed old cookie ended the new session")
	}
}
