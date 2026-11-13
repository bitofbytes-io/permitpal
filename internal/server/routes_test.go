package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
	"github.com/drywaters/permitpal/internal/repository"
)

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
	assertDecimalHourInput(t, body, "total_hours", `(1000(\.0)?|[0-9]{1,3}(\.[0-9])?|\.[0-9])`)
	assertDecimalHourInput(t, body, "night_hours", `(1000(\.0)?|[0-9]{1,3}(\.[0-9])?|\.[0-9])`)

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

type unreachableStore struct{ repository.Store }

func (unreachableStore) Ping(context.Context) error { return errors.New("connection refused") }

func TestHealthAndReadySkipAuth(t *testing.T) {
	for name, tc := range map[string]struct {
		store      func(repository.Store) repository.Store
		path       string
		wantStatus int
		wantBody   string
	}{
		"health":         {func(s repository.Store) repository.Store { return s }, "/health", http.StatusOK, "ok"},
		"ready":          {func(s repository.Store) repository.Store { return s }, "/ready", http.StatusOK, `{"status":"ok","db":"ok"}`},
		"health db down": {func(s repository.Store) repository.Store { return unreachableStore{s} }, "/health", http.StatusOK, "ok"},
		"ready db down":  {func(s repository.Store) repository.Store { return unreachableStore{s} }, "/ready", http.StatusServiceUnavailable, `{"status":"degraded","db":"error"}`},
	} {
		t.Run(name, func(t *testing.T) {
			app := newTestApp(t)
			app.store = tc.store(app.store)
			ts := httptest.NewServer(app.Router())
			defer ts.Close()
			// No session cookie and no redirects: the probe must answer directly.
			client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			res := doGet(t, client, ts.URL+tc.path)
			if body := strings.TrimSpace(readBody(t, res)); res.StatusCode != tc.wantStatus || body != tc.wantBody {
				t.Fatalf("GET %s: status=%d body=%q; want %d %s", tc.path, res.StatusCode, body, tc.wantStatus, tc.wantBody)
			}
		})
	}
}

func TestHTTPServerSetsEveryTimeout(t *testing.T) {
	srv := newTestApp(t).HTTPServer()
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.WriteTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Fatalf("timeouts: header=%v read=%v write=%v idle=%v", srv.ReadHeaderTimeout, srv.ReadTimeout, srv.WriteTimeout, srv.IdleTimeout)
	}
	if srv.WriteTimeout <= requestTimeout {
		t.Fatalf("write timeout %v must outlast the request timeout %v", srv.WriteTimeout, requestTimeout)
	}
}

// deadlineStore records the deadline of the context each tracker load receives.
type deadlineStore struct {
	repository.Store
	deadline time.Time
}

func (s *deadlineStore) GetTracker(ctx context.Context, driverID int64) (model.Tracker, error) {
	s.deadline, _ = ctx.Deadline()
	return s.Store.GetTracker(ctx, driverID)
}

func TestRequestContextHasATimeout(t *testing.T) {
	app := newTestApp(t)
	store := &deadlineStore{Store: app.store}
	app.store = store
	ts := httptest.NewServer(app.Router())
	defer ts.Close()
	client := ts.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	start := time.Now()
	body := readBody(t, doPostForm(t, client, ts.URL+"/login", url.Values{"username": {"aiden"}, "password": {"test-password"}}))
	if !strings.Contains(body, "Road Test Skill Checklist") {
		t.Fatal("login did not reach the dashboard")
	}
	if store.deadline.IsZero() || store.deadline.After(start.Add(requestTimeout+time.Second)) {
		t.Fatalf("store query deadline = %v, want about %v after the request started", store.deadline, requestTimeout)
	}
}
