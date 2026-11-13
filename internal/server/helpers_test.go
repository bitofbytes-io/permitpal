package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/config"
	"github.com/drywaters/permitpal/internal/model"
	"github.com/drywaters/permitpal/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

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
	if _, err := store.UpdateProfile(context.Background(), caleb.ID, model.Profile{TotalHours: 56, NightHours: 8}, time.Now()); err != nil {
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
