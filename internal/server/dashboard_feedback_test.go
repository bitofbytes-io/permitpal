package server

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
	"github.com/drywaters/permitpal/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDashboardSaveFeedbackInBrowser drives the dashboard forms in Chromium
// with the bundled htmx, against the memory store and, when
// PERMITPAL_TEST_DATABASE_URL names a disposable database, PostgreSQL. It
// needs Node and Playwright, so it runs only when PERMITPAL_PLAYWRIGHT_MODULE
// names the Playwright module to require.
func TestDashboardSaveFeedbackInBrowser(t *testing.T) {
	if os.Getenv("PERMITPAL_PLAYWRIGHT_MODULE") == "" {
		t.Skip("PERMITPAL_PLAYWRIGHT_MODULE is not set; set it to a Playwright module path to run the browser test")
	}
	t.Chdir("../..") // The page loads htmx from ./static.
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			app := newTestApp(t)
			if backend == "postgres" {
				app.store = browserPostgresStore(t)
			}
			ts := httptest.NewServer(app.Router())
			defer ts.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			output, err := exec.CommandContext(ctx, "node", "internal/server/testdata/dashboard_feedback.cjs", ts.URL).CombinedOutput()
			if err != nil {
				t.Fatalf("browser check failed: %v\n%s", err, output)
			}

			// The store holds what the browser saw last: the update sent after
			// the held clear, and the corrected progress.
			driver, err := app.store.DriverByUsername(ctx, "aiden")
			if err != nil {
				t.Fatal(err)
			}
			tracker, err := app.store.GetTracker(ctx, driver.ID)
			if err != nil {
				t.Fatal(err)
			}
			req, ok := model.RequirementByKey(tracker.Requirements, "use-of-lane")
			if !ok || req.Rating != model.RatingFair || model.DateValue(req.RatedOn) != "2025-05-01" || req.Notes != "After clear" {
				t.Fatalf("saved use-of-lane = %+v, want fair, 2025-05-01, \"After clear\"", req)
			}
			if len(tracker.Profile.HourEntries) != 6 {
				t.Fatalf("journal has %d entries, want six changed saves; duplicate/permit-only/invalid saves must add none", len(tracker.Profile.HourEntries))
			}
			if tracker.Profile.TotalHours != 6 || tracker.Profile.NightHours != 2 {
				t.Fatalf("saved profile = %+v, want 6 total and 2 night hours", tracker.Profile)
			}
		})
	}
}

// browserPostgresStore returns a PostgresStore in a new schema of the
// disposable database named by PERMITPAL_TEST_DATABASE_URL, set up with the
// repository's migrations and dropped after the test. It expects the
// working directory to be the repository root.
func browserPostgresStore(t *testing.T) repository.Store {
	t.Helper()
	databaseURL := os.Getenv("PERMITPAL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PERMITPAL_TEST_DATABASE_URL is not set; skipping the PostgreSQL browser run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("permitpal_test_%x", random)
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migrations, err := filepath.Glob("migrations/*.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	for _, path := range migrations {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		up, _, found := strings.Cut(string(migration), "-- +goose Down")
		if !found {
			t.Fatalf("migration %s has no down boundary", path)
		}
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", path, err)
		}
	}
	return repository.NewPostgresStore(pool)
}

func TestDashboardScriptsCarryTheCSPNonce(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	client := ts.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	closeBody(t, doPostForm(t, client, ts.URL+"/login", url.Values{"username": {"aiden"}, "password": {"test-password"}}))
	res := doGet(t, client, ts.URL+"/")
	csp := res.Header.Get("Content-Security-Policy")
	body := readBody(t, res)
	_, rest, ok := strings.Cut(csp, "'nonce-")
	nonce, _, _ := strings.Cut(rest, "'")
	if !ok || nonce == "" {
		t.Fatalf("CSP has no nonce: %q", csp)
	}
	// Scripts with a src must be same-origin; every inline script must carry
	// this response's nonce.
	scriptTag := regexp.MustCompile(`(?i)<script\b([^>]*)>`)
	attr := regexp.MustCompile(`([a-zA-Z-]+)(?:="([^"]*)")?`)
	inline := 0
	for _, tag := range scriptTag.FindAllStringSubmatch(body, -1) {
		attrs := map[string]string{}
		for _, a := range attr.FindAllStringSubmatch(tag[1], -1) {
			attrs[strings.ToLower(a[1])] = a[2]
		}
		if src, ok := attrs["src"]; ok {
			if !strings.HasPrefix(src, "/") || strings.HasPrefix(src, "//") {
				t.Fatalf("script %s does not load from this origin", tag[0])
			}
			continue
		}
		inline++
		if attrs["nonce"] != nonce {
			t.Fatalf("inline script %s does not carry nonce %q", tag[0], nonce)
		}
	}
	if inline == 0 {
		t.Fatal("dashboard rendered no inline scripts to check")
	}
}

// The cap can exclude most recorded hours, so cumulative inputs must remain
// usable after either credited requirement has been exceeded in raw totals.
func TestCumulativeHourCeilingInBrowser(t *testing.T) {
	if os.Getenv("PERMITPAL_PLAYWRIGHT_MODULE") == "" {
		t.Skip("PERMITPAL_PLAYWRIGHT_MODULE is not set")
	}
	t.Chdir("../..")
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			app := newTestApp(t)
			if backend == "postgres" {
				app.store = browserPostgresStore(t)
			}
			now := time.Now().In(app.cfg.Location)
			driver, err := app.store.EnsureDriver(t.Context(), "aiden", now)
			if err != nil {
				t.Fatal(err)
			}
			issue := now.AddDate(-2, 0, 0)
			if _, err = app.store.UpdateProfile(t.Context(), driver.ID, model.Profile{PermitIssueDate: &issue, TotalHours: 60, NightHours: 20}, now.AddDate(0, 0, -7)); err != nil {
				t.Fatal(err)
			}
			ts := httptest.NewServer(app.Router())
			defer ts.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			output, err := exec.CommandContext(ctx, "node", "internal/server/testdata/dashboard_feedback.cjs", ts.URL, "cumulative-limit").CombinedOutput()
			if err != nil {
				t.Fatalf("browser cumulative limit: %v\n%s", err, output)
			}
			tracker, err := app.store.GetTracker(ctx, driver.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tracker.Profile.TotalHours != 61 || tracker.Profile.NightHours != 21 || len(tracker.Profile.HourEntries) != 4 {
				t.Fatalf("saved progress=%+v", tracker.Profile)
			}
			if credit := model.CreditHours(tracker.Profile); credit.TotalHours != 11 || credit.NightHours != 11 {
				t.Fatalf("credit=%+v", credit)
			}
		})
	}
}
