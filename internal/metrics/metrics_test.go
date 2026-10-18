package metrics

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/drivers/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Route("/requirements", func(r chi.Router) {
		r.Post("/{key}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnprocessableEntity) })
	})
	return r
}

func serve(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), method, target, nil))
	return rec
}

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := serve(t, h, http.MethodGet, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want 200", rec.Code)
	}
	return rec.Body.String()
}

func TestMetricsExposeREDAndDomainSeriesWithRoutePatterns(t *testing.T) {
	router := testRouter()
	serve(t, router, http.MethodGet, "/drivers/42?token=secret")
	serve(t, router, http.MethodGet, "/drivers/43")
	serve(t, router, http.MethodPost, "/requirements/parallel-parking")
	serve(t, router, http.MethodGet, "/no/such/page")
	serve(t, router, "BREW", "/drivers/44")
	serve(t, router, http.MethodGet, "/health")
	LoginFailure(LoginInvalidPassword)

	body := scrape(t, Handler(NewRegistry()))
	for _, want := range []string{
		`http_requests_total{code="200",method="GET",route="/drivers/{id}"} 2`,
		`http_requests_total{code="422",method="POST",route="/requirements/{key}"} 1`,
		`http_requests_total{code="404",method="GET",route="unmatched"} 1`,
		`route="/drivers/{id}"`,
		`method="OTHER"`,
		`http_request_duration_seconds_bucket{code="200",method="GET",route="/drivers/{id}",le="0.005"}`,
		`http_request_duration_seconds_count{code="200",method="GET",route="/drivers/{id}"} 2`,
		`permitpal_login_failures_total{reason="invalid_password"} 1`,
		`permitpal_login_failures_total{reason="rate_limited"} 0`,
		"go_goroutines ",
		"process_start_time_seconds ",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics is missing %q", want)
		}
	}
	for _, raw := range []string{"/drivers/42", "/drivers/43", "parallel-parking", "/no/such/page", "token", "/health"} {
		if strings.Contains(body, raw) {
			t.Errorf("/metrics contains raw path or query %q", raw)
		}
	}
}

func TestHandlerServesOnlyMetrics(t *testing.T) {
	h := Handler(NewRegistry())
	if rec := serve(t, h, http.MethodGet, "/"); rec.Code != http.StatusNotFound {
		t.Fatalf("GET / = %d, want 404", rec.Code)
	}
	if rec := serve(t, h, http.MethodPost, "/metrics"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /metrics = %d, want 405", rec.Code)
	}
}

func TestPoolCollectorExportsPoolStats(t *testing.T) {
	// pgxpool connects lazily, so Stat works without a database.
	pool, err := pgxpool.New(context.Background(), "postgres://permitpal@127.0.0.1:1/permitpal?pool_max_conns=7")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	body := scrape(t, Handler(NewRegistry(NewPoolCollector(pool))))
	for _, want := range []string{
		"pgxpool_max_conns 7",
		"pgxpool_acquired_conns 0",
		"pgxpool_idle_conns 0",
		"pgxpool_total_conns 0",
		"pgxpool_empty_acquire_wait_seconds_total 0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics is missing %q", want)
		}
	}
	if n, err := testutil.GatherAndCount(NewRegistry(NewPoolCollector(pool)), "pgxpool_acquires_total"); err != nil || n != 1 {
		t.Fatalf("pgxpool_acquires_total series = %d, %v; want 1", n, err)
	}
}

func TestStartServesMetricsAndShutsDown(t *testing.T) {
	srv := Start("127.0.0.1:0", NewRegistry())
	if srv == nil {
		t.Fatal("Start returned nil for a free address")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Get("http://" + srv.Addr().String() + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "permitpal_login_failures_total") {
		t.Fatalf("GET /metrics = %d: %.200s", res.StatusCode, body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get("http://" + srv.Addr().String() + "/metrics"); err == nil {
		t.Fatal("metrics listener still serving after Shutdown")
	}
}

func TestStartDisabledOrUnavailableReturnsNil(t *testing.T) {
	if srv := Start(Disabled, NewRegistry()); srv != nil {
		t.Fatal("Start(off) returned a server")
	}

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	srv := Start(taken.Addr().String(), NewRegistry())
	if srv != nil {
		t.Fatal("Start on a bound address returned a server")
	}
	// A nil server shuts down cleanly, so callers need no special case.
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
