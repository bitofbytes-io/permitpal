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
			load := func() model.Tracker {
				t.Helper()
				tracker, err := store.GetTracker(context.Background(), driver.ID)
				if err != nil {
					t.Fatal(err)
				}
				return tracker
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
			// "now" is 2026-05-01 in the app time zone.
			for name, test := range map[string]struct {
				path string
				form url.Values
				want string
			}{
				"night-over-total":     {"/profile", url.Values{"total_hours": {"4"}, "night_hours": {"4.5"}}, "Night hours cannot be more than total hours"},
				"future-permit-date":   {"/profile", url.Values{"total_hours": {"4"}, "night_hours": {"1"}, "permit_issue_date": {"2026-05-02"}}, "Permit issue date cannot be in the future"},
				"future-rated-on-date": {"/requirements/quick-stop", url.Values{"rating": {"good"}, "rated_on": {"2026-05-02"}}, "Last rated date cannot be in the future"},
			} {
				t.Run(name, func(t *testing.T) {
					before := load()
					rec := submit(test.path, test.form)
					if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), test.want) {
						t.Fatalf("status=%d body=%q, want 400 %q", rec.Code, rec.Body.String(), test.want)
					}
					if !reflect.DeepEqual(before, load()) {
						t.Fatal("rejected input changed the tracker")
					}
				})
			}
			t.Run("clearing-a-saved-future-date", func(t *testing.T) {
				// A rating saved before future dates were rejected can still be cleared.
				req, _ := model.RequirementByKey(load().Requirements, "turn-about")
				future := now.AddDate(0, 0, 3)
				req.Rating, req.RatedOn = model.RatingGood, &future
				if _, err := store.UpdateRequirement(context.Background(), driver.ID, req); err != nil {
					t.Fatal(err)
				}
				rec := submit("/requirements/turn-about", url.Values{"rating": {"good"}, "rated_on": {future.Format("2006-01-02")}, "clear_rating": {"true"}})
				if rec.Code != http.StatusOK {
					t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
				}
				if cleared, _ := model.RequirementByKey(load().Requirements, "turn-about"); cleared.Rating != model.RatingNotRated || cleared.RatedOn != nil {
					t.Fatalf("requirement not cleared: %+v", cleared)
				}
			})
			t.Run("today-and-equal-hours-are-allowed", func(t *testing.T) {
				if rec := submit("/profile", url.Values{"total_hours": {"4"}, "night_hours": {"4"}, "permit_issue_date": {"2026-05-01"}}); rec.Code != http.StatusOK {
					t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
				}
				if rec := submit("/requirements/quick-stop", url.Values{"rating": {"good"}, "rated_on": {"2026-05-01"}}); rec.Code != http.StatusOK {
					t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
				}
			})
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
		up, down, found := strings.Cut(string(migration), "-- +goose Down")
		if !found {
			t.Fatalf("migration %s has no down boundary", path)
		}
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", path, err)
		}
		if filepath.Base(path) != "003_multi_driver_ratings.sql" {
			continue
		}
		// Exercise the exact rollback SQL in the same non-public schema, then
		// restore it, before later migrations build on 003's tables.
		beforeRows := migrationSnapshot(t, pool)
		for _, sql := range []string{down, up} {
			if _, err := pool.Exec(ctx, sql); err != nil {
				t.Fatalf("migration 003 down/up: %v", err)
			}
		}
		if afterRows := migrationSnapshot(t, pool); !reflect.DeepEqual(beforeRows, afterRows) {
			t.Fatal("migration 003 down/up changed Caleb history")
		}
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
			before, err := store.GetTracker(ctx, caleb.ID)
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
				dash, err := store.GetTracker(ctx, aiden.ID)
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
			after, err := store.GetTracker(ctx, caleb.ID)
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
			dash, err := store.GetTracker(ctx, first.ID)
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
			dash, err := store.GetTracker(ctx, driver.ID)
			if err != nil {
				t.Fatal(err)
			}
			req := dash.Requirements[0]
			if req.Rating != model.RatingNotRated || req.RatedOn != nil || req.Notes != "Keep this practice note" {
				t.Fatalf("cleared requirement=%+v", req)
			}
			if focus := model.NewDashboard(driver, dash.Profile, dash.Requirements, now).PracticeFocus; len(focus) == 0 || focus[0].Key != "quick-stop" {
				t.Fatal("clear did not restore practice focus")
			}
			if strings.Contains(rec.Body.String(), `checked`) || !strings.Contains(rec.Body.String(), `hx-swap-oob="outerHTML"`) || !strings.Contains(rec.Body.String(), "Clear rating for Quick stop") {
				t.Fatal("clear response did not reset selection and refresh focus")
			}
		})
	}
}

func TestEndSessionsAdvancesTheGenerationOnce(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			now := time.Now()
			store := validationStore(t, backend, now)
			created, err := store.EnsureDriver(ctx, "aiden", now)
			if err != nil {
				t.Fatal(err)
			}
			if created.SessionGeneration != 0 {
				t.Fatalf("new driver generation = %d", created.SessionGeneration)
			}
			for _, generation := range []int64{0, 0, 5} {
				if err := store.EndSessions(ctx, "aiden", generation); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.EndSessions(ctx, "missing", 0); err != nil {
				t.Fatal(err)
			}
			byName, err := store.DriverByUsername(ctx, "aiden")
			if err != nil {
				t.Fatal(err)
			}
			again, err := store.EnsureDriver(ctx, "aiden", now)
			if err != nil {
				t.Fatal(err)
			}
			if byName.SessionGeneration != 1 || again.SessionGeneration != 1 {
				t.Fatalf("generation = %d / %d, want 1 after one matching logout", byName.SessionGeneration, again.SessionGeneration)
			}
		})
	}
}
