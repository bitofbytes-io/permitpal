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
			handler := NewDashboardHandler(store)
			handler.now = func() time.Time { return now }
			router := chi.NewRouter()
			router.Post("/profile", handler.UpdateProfile)
			router.Post("/requirements/{key}", handler.UpdateRequirement)
			submit := func(path string, form url.Values) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return rec
			}
			load := func() model.Dashboard {
				t.Helper()
				dashboard, err := store.GetDashboard(context.Background(), now)
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
				for _, status := range []string{"mastered", "needs_practice"} {
					t.Run("invalid-requirement-date/"+status+"/"+date, func(t *testing.T) {
						before := load()
						rec := submit("/requirements/starting-the-car", url.Values{"status": {status}, "notes": {"must not be saved"}, "mastered_date": {date}})
						if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Mastered date") {
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
					rec = submit("/requirements/starting-the-car", url.Values{"status": {"mastered"}, "notes": {"updated"}, "mastered_date": {date}})
					if rec.Code != http.StatusOK {
						t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
					}
					requirement, ok := model.RequirementByKey(load().Requirements, "starting-the-car")
					if !ok || requirement.Status != model.StatusMastered || requirement.Notes != "updated" || model.DateValue(requirement.MasteredDate) != strings.TrimSpace(date) {
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
	return repository.NewPostgresStore(pool)
}
