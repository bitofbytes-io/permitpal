package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/metrics"
	"github.com/drywaters/permitpal/internal/repository"
)

func TestPublicRouterRecordsRoutePatternsAndLoginFailures(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	scrapeHandler := metrics.Handler(metrics.NewRegistry())

	const (
		requirement   = `http_requests_total{code="303",method="POST",route="/requirements/{key}"}`
		login         = `http_requests_total{code="200",method="POST",route="/login"}`
		unmatched     = `http_requests_total{code="404",method="GET",route="unmatched"}`
		wrongPassword = `permitpal_login_failures_total{reason="invalid_password"}`
		badUsername   = `permitpal_login_failures_total{reason="invalid_username"}`
	)
	before := scrapeMetrics(t, scrapeHandler)

	// Unauthenticated, so RequireAuth redirects; the route is still the pattern.
	closeBody(t, doPostForm(t, noRedirectClient(ts), ts.URL+"/requirements/parallel-parking", url.Values{}))
	closeBody(t, doPostForm(t, ts.Client(), ts.URL+"/login", url.Values{"username": {"aiden"}, "password": {"wrong"}}))
	closeBody(t, doPostForm(t, ts.Client(), ts.URL+"/login", url.Values{"username": {"Not A User!"}, "password": {"wrong"}}))
	closeBody(t, doGet(t, ts.Client(), ts.URL+"/drivers/aiden?secret=1"))
	closeBody(t, doGet(t, ts.Client(), ts.URL+"/health"))
	closeBody(t, doGet(t, ts.Client(), ts.URL+"/ready"))

	after := scrapeMetrics(t, scrapeHandler)
	for series, delta := range map[string]float64{
		requirement:   1,
		login:         2,
		unmatched:     1,
		wrongPassword: 1,
		badUsername:   1,
	} {
		if got := seriesValue(after, series) - seriesValue(before, series); got != delta {
			t.Errorf("%s increased by %v, want %v", series, got, delta)
		}
	}
	for _, raw := range []string{"parallel-parking", "/drivers/aiden", "secret", `route="/health"`, `route="/ready"`} {
		if strings.Contains(after, raw) {
			t.Errorf("/metrics contains %q", raw)
		}
	}
}

// TestPostgresPoolMetrics exports the pool of a real PostgresStore, when
// PERMITPAL_TEST_DATABASE_URL names a disposable database.
func TestPostgresPoolMetrics(t *testing.T) {
	t.Chdir("../..") // browserPostgresStore reads ./migrations.
	store := browserPostgresStore(t)
	pg, ok := store.(*repository.PostgresStore)
	if !ok {
		t.Fatalf("store is %T, want *repository.PostgresStore", store)
	}
	if _, err := pg.EnsureDriver(t.Context(), "aiden", time.Now()); err != nil {
		t.Fatal(err)
	}

	body := scrapeMetrics(t, metrics.Handler(metrics.NewRegistry(metrics.NewPoolCollector(pg.Pool()))))
	if got := seriesValue(body, "pgxpool_total_conns"); got < 1 {
		t.Errorf("pgxpool_total_conns = %v, want at least 1", got)
	}
	if got := seriesValue(body, "pgxpool_acquires_total"); got < 1 {
		t.Errorf("pgxpool_acquires_total = %v, want at least 1", got)
	}
	if got, want := seriesValue(body, "pgxpool_max_conns"), float64(pg.Pool().Stat().MaxConns()); got != want {
		t.Errorf("pgxpool_max_conns = %v, want %v", got, want)
	}
}

func TestPublicRouterDoesNotServeMetrics(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	res := doGet(t, ts.Client(), ts.URL+"/metrics")
	body := readBody(t, res)
	if res.StatusCode != http.StatusNotFound || strings.Contains(body, "http_requests_total") {
		t.Fatalf("public GET /metrics = %d: %.200s", res.StatusCode, body)
	}
}

func noRedirectClient(ts *httptest.Server) *http.Client {
	client := *ts.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}

func scrapeMetrics(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d", rec.Code)
	}
	return rec.Body.String()
}

// seriesValue returns the sample value of series in a text exposition, or 0
// when the series is absent.
func seriesValue(body, series string) float64 {
	for _, line := range strings.Split(body, "\n") {
		if value, ok := strings.CutPrefix(line, series+" "); ok {
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return -1
			}
			return parsed
		}
	}
	return 0
}
