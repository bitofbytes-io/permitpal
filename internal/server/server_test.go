package server

import (
	"context"
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

	res := doGet(t, http.DefaultClient, ts.URL+"/login")
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
	loginForm := url.Values{"password": {"permitpal"}}
	res := doPostForm(t, client, ts.URL+"/login", loginForm)
	closeBody(t, res)

	res = doGet(t, client, ts.URL+"/")
	body := readBody(t, res)
	if !strings.Contains(body, "Skill Mastery Checklist") || !strings.Contains(body, "Lane changes") {
		t.Fatalf("dashboard missing checklist content: %s", body)
	}
	assertDecimalHourInput(t, body, "total_hours", `(60(\.0)?|[0-5]?[0-9](\.[0-9])?|\.[0-9])`)
	assertDecimalHourInput(t, body, "night_hours", `(10(\.0)?|[0-9](\.[0-9])?|\.[0-9])`)

	update := url.Values{
		"status":        {"mastered"},
		"mastered_date": {"2026-05-02"},
		"notes":         {"Clean mirror checks."},
	}
	res = doPostForm(t, client, ts.URL+"/requirements/lane-changes", update)
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
	res = doPostForm(t, client, ts.URL+"/profile", progress)
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
		Password:        "permitpal",
		SessionSecret:   "test-secret",
		SessionCookie:   "permitpal_session",
		DefaultUsername: "driver",
	}
	store := repository.NewMemoryStore(time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local))
	app := New(cfg, store, slog.Default())
	return httptest.NewServer(app.Router())
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
