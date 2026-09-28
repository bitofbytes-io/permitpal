package handler

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/middleware"
	"github.com/drywaters/permitpal/internal/model"
	"github.com/drywaters/permitpal/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDashboardValidation(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
			store := validationStore(t, backend, now)
			driver, err := store.EnsureDriver(context.Background(), "aiden", now)
			if err != nil {
				t.Fatal(err)
			}
			handler := NewDashboardHandler(store, time.Local)
			handler.now = func() time.Time { return now }
			router := chi.NewRouter()
			router.Post("/profile", handler.UpdateProfile)
			router.Post("/requirements/{key}", handler.UpdateRequirement)
			submit := func(path string, form url.Values) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req.WithContext(middleware.WithDriver(req.Context(), driver)))
				return rec
			}
			load := func() model.Dashboard {
				t.Helper()
				dashboard, err := store.GetDashboard(context.Background(), driver, now)
				if err != nil {
					t.Fatal(err)
				}
				return dashboard
			}
			for _, date := range []string{"not-a-date", "2026-02-30", "2026-13-01", "2026-01-15T12:00:00Z"} {
				t.Run("invalid-profile-date/"+date, func(t *testing.T) {
					before := load()
					rec := submit("/profile", url.Values{"total_hours": {"12.5"}, "night_hours": {"2.0"}, "permit_issue_date": {date}})
					if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Permit issue date") {
						t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
					}
					if !reflect.DeepEqual(before, load()) {
						t.Fatal("invalid date changed stored progress")
					}
				})
				for _, status := range []string{"good", "fair"} {
					t.Run("invalid-requirement-date/"+status+"/"+date, func(t *testing.T) {
						before := load()
						rec := submit("/requirements/quick-stop", url.Values{"rating": {status}, "notes": {"must not be saved"}, "rated_on": {date}})
						if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Last rated date") {
							t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
						}
						if !reflect.DeepEqual(before, load()) {
							t.Fatal("invalid date changed stored requirement")
						}
					})
				}
			}
			for _, field := range []string{"total_hours", "night_hours"} {
				for _, value := range []string{"NaN", "Inf", "+Inf", "-Inf", "Infinity", "1.23", "-1", "61"} {
					t.Run(field+"/"+value, func(t *testing.T) {
						before := load()
						form := url.Values{"total_hours": {"12.5"}, "night_hours": {"2.0"}, "permit_issue_date": {"2026-01-15"}}
						form.Set(field, value)
						rec := submit("/profile", form)
						if rec.Code != http.StatusBadRequest {
							t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
						}
						if !reflect.DeepEqual(before, load()) {
							t.Fatal("invalid hours changed stored progress")
						}
					})
				}
			}
			for _, date := range []string{"2026-01-15", "", "  "} {
				t.Run("valid-or-cleared-date/"+date, func(t *testing.T) {
					rec := submit("/profile", url.Values{"total_hours": {"12.5"}, "night_hours": {"2.0"}, "permit_issue_date": {date}})
					if rec.Code != http.StatusOK {
						t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
					}
					profile := load().Profile
					if model.DateValue(profile.PermitIssueDate) != strings.TrimSpace(date) || profile.TotalHours != 12.5 || profile.NightHours != 2 {
						t.Fatalf("unexpected saved profile: %+v", profile)
					}
					rec = submit("/requirements/quick-stop", url.Values{"rating": {"good"}, "notes": {"updated"}, "rated_on": {date}})
					if rec.Code != http.StatusOK {
						t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
					}
					requirement, ok := model.RequirementByKey(load().Requirements, "quick-stop")
					if !ok || requirement.Rating != model.RatingGood || requirement.Notes != "updated" || model.DateValue(requirement.RatedOn) != expectedRatedDate(date, now) {
						t.Fatalf("unexpected saved requirement: %+v", requirement)
					}
				})
			}
		})
	}
}

// Each database test uses its own schema and the production migrations. The
// explicitly supplied test URL must point to a disposable PostgreSQL database.
func validationStore(t *testing.T, backend string, now time.Time) repository.Store {
	t.Helper()
	if backend == "memory" {
		return repository.NewMemoryStore(now)
	}
	databaseURL := os.Getenv("PERMITPAL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PERMITPAL_TEST_DATABASE_URL is not set")
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
	migrations, err := filepath.Glob("../../migrations/*.sql")
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
	// Exercise the exact rollback SQL in the same non-public schema, then restore it.
	migration, err := os.ReadFile("../../migrations/003_multi_driver_ratings.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down, _ := strings.Cut(string(migration), "-- +goose Down")
	beforeRows := migrationSnapshot(t, pool)
	for _, sql := range []string{down, up} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("migration 003 down/up: %v", err)
		}
	}
	if afterRows := migrationSnapshot(t, pool); !reflect.DeepEqual(beforeRows, afterRows) {
		t.Fatal("migration 003 down/up changed Caleb history")
	}
	return repository.NewPostgresStore(pool)
}

func expectedRatedDate(date string, now time.Time) string {
	if strings.TrimSpace(date) == "" {
		return now.Format("2006-01-02")
	}
	return strings.TrimSpace(date)
}

func TestDriverIsolationAndRatings(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
			store := validationStore(t, backend, now)
			aiden, err := store.EnsureDriver(ctx, "aiden", now)
			if err != nil {
				t.Fatal(err)
			}
			caleb, err := store.EnsureDriver(ctx, "caleb", now)
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.GetDashboard(ctx, caleb, now)
			if err != nil {
				t.Fatal(err)
			}
			handler := NewDashboardHandler(store, time.Local)
			handler.now = func() time.Time { return now }
			router := chi.NewRouter()
			router.Post("/requirements/{key}", handler.UpdateRequirement)
			router.Post("/profile", handler.UpdateProfile)
			submit := func(path string, form url.Values) *httptest.ResponseRecorder {
				req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req = req.WithContext(middleware.WithDriver(req.Context(), aiden))
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return rec
			}
			for _, rating := range []string{"mastered", "needs_practice", "great", "BAD"} {
				if rec := submit("/requirements/quick-stop", url.Values{"rating": {rating}}); rec.Code != 400 {
					t.Fatalf("accepted %q: %d", rating, rec.Code)
				}
			}
			for _, rating := range []string{"bad", "fair", "good", "not_rated", ""} {
				rec := submit("/requirements/quick-stop", url.Values{"rating": {rating}, "notes": {"Aiden only"}})
				if rec.Code != 200 {
					t.Fatalf("save %q: %d %s", rating, rec.Code, rec.Body.String())
				}
				if !strings.Contains(rec.Body.String(), `id="practice-focus"`) || !strings.Contains(rec.Body.String(), `hx-swap-oob="outerHTML"`) {
					t.Fatal("response did not update practice focus out of band")
				}
				dash, err := store.GetDashboard(ctx, aiden, now)
				if err != nil {
					t.Fatal(err)
				}
				req := dash.Requirements[0]
				if rating == "" || rating == "not_rated" {
					if req.RatedOn != nil {
						t.Fatal("not rated kept date")
					}
				} else if model.DateValue(req.RatedOn) != "2026-05-01" {
					t.Fatal("rating missing today's date")
				}
			}
			if rec := submit("/profile", url.Values{"total_hours": {"20"}, "night_hours": {"3"}}); rec.Code != 200 {
				t.Fatal(rec.Body.String())
			}
			after, err := store.GetDashboard(ctx, caleb, now)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("Aiden update changed Caleb")
			}
			if backend == "postgres" {
				if len(after.Requirements) != 13 || after.Profile.TotalHours != 34.5 || after.Profile.NightHours != 6 || after.Requirements[0].Rating != model.RatingGood || model.DateValue(after.Requirements[0].RatedOn) != "2026-03-08" {
					t.Fatalf("Caleb migration failed: %+v", after)
				}
			}
		})
	}
}

func migrationSnapshot(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(context.Background(), `select json_build_object('profile',(select row_to_json(p) from (select permit_issue_date,total_hours,night_hours,updated_at from app_profile) p),'requirements',(select json_agg(r order by sort_order) from (select key,title,description,status,rated_on,notes,sort_order,updated_at from requirement_items) r))::text`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestConcurrentEnsureDriver(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			now := time.Now()
			store := validationStore(t, backend, now)
			ctx := context.Background()
			type result struct {
				driver model.Driver
				err    error
			}
			results := make(chan result, 8)
			for i := 0; i < 8; i++ {
				go func() { driver, err := store.EnsureDriver(ctx, "aiden", now); results <- result{driver, err} }()
			}
			var first model.Driver
			for i := 0; i < 8; i++ {
				result := <-results
				if result.err != nil {
					t.Fatal(result.err)
				}
				if i == 0 {
					first = result.driver
				}
				if result.driver != first {
					t.Fatal("concurrent login created different drivers")
				}
			}
			dash, err := store.GetDashboard(ctx, first, now)
			if err != nil {
				t.Fatal(err)
			}
			if len(dash.Requirements) != 17 || dash.Profile.TotalHours != 0 {
				t.Fatal("concurrent login did not create one complete fresh tracker")
			}
		})
	}
}

func TestClearRatingPreservesNotesAndUpdatesFocus(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			now := time.Now()
			ctx := context.Background()
			store := validationStore(t, backend, now)
			driver, err := store.EnsureDriver(ctx, "aiden", now)
			if err != nil {
				t.Fatal(err)
			}
			handler := NewDashboardHandler(store, time.Local)
			router := chi.NewRouter()
			router.Post("/requirements/{key}", handler.UpdateRequirement)
			submit := func(form url.Values) *httptest.ResponseRecorder {
				req := httptest.NewRequest("POST", "/requirements/quick-stop", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req = req.WithContext(middleware.WithDriver(req.Context(), driver))
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return rec
			}
			form := url.Values{"rating": {"good"}, "rated_on": {"2026-05-01"}, "notes": {"Keep this practice note"}}
			if rec := submit(form); rec.Code != 200 {
				t.Fatal(rec.Body.String())
			}
			form.Set("clear_rating", "true")
			rec := submit(form)
			if rec.Code != 200 {
				t.Fatal(rec.Body.String())
			}
			dash, err := store.GetDashboard(ctx, driver, now)
			if err != nil {
				t.Fatal(err)
			}
			req := dash.Requirements[0]
			if req.Rating != model.RatingNotRated || req.RatedOn != nil || req.Notes != "Keep this practice note" {
				t.Fatalf("cleared requirement=%+v", req)
			}
			if len(dash.PracticeFocus) == 0 || dash.PracticeFocus[0].Key != "quick-stop" {
				t.Fatal("clear did not restore practice focus")
			}
			if strings.Contains(rec.Body.String(), `checked`) || !strings.Contains(rec.Body.String(), `hx-swap-oob="outerHTML"`) || !strings.Contains(rec.Body.String(), "Clear rating for Quick stop") {
				t.Fatal("clear response did not reset selection and refresh focus")
			}
		})
	}
}
