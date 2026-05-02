package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/config"
	"github.com/drywaters/permitpal/internal/repository"
)

func TestUnauthenticatedUserSeesLogin(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body := readBody(t, res)
	if !strings.Contains(body, "PermitPal") || !strings.Contains(body, "Sign in") {
		t.Fatalf("login page did not render expected copy: %s", body)
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
	loginForm := url.Values{"password": {"test-password"}}
	res, err := client.PostForm(ts.URL+"/login", loginForm)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	res, err = client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if !strings.Contains(body, "Skill Mastery Checklist") || !strings.Contains(body, "Lane changes") {
		t.Fatalf("dashboard missing checklist content: %s", body)
	}

	update := url.Values{
		"status":        {"mastered"},
		"mastered_date": {"2026-05-02"},
		"notes":         {"Clean mirror checks."},
	}
	res, err = client.PostForm(ts.URL+"/requirements/lane-changes", update)
	if err != nil {
		t.Fatal(err)
	}
	row := readBody(t, res)
	if !strings.Contains(row, "Mastered") || !strings.Contains(row, "Clean mirror checks.") {
		t.Fatalf("requirement partial missing updated content: %s", row)
	}

	progress := url.Values{
		"total_hours":        {"42.5"},
		"night_hours":        {"8.5"},
		"permit_issue_date":  {"2025-10-18"},
		"unexpected_ignored": {"x"},
	}
	res, err = client.PostForm(ts.URL+"/profile", progress)
	if err != nil {
		t.Fatal(err)
	}
	panel := readBody(t, res)
	if !strings.Contains(panel, "42.5 total hours") || !strings.Contains(panel, "8.5 night hours") {
		t.Fatalf("progress partial missing updated values: %s", panel)
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := &config.Config{
		AppEnv:          "development",
		DataStore:       config.DataStoreMemory,
		Port:            "4600",
		Password:        "test-password",
		SessionSecret:   "test-secret",
		SessionCookie:   "permitpal_session",
		DefaultUsername: "driver",
	}
	store := repository.NewMemoryStore(time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local))
	app := New(cfg, store, slog.Default())
	return httptest.NewServer(app.Router())
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
