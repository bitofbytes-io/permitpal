package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/middleware"
	"github.com/drywaters/permitpal/internal/model"
	"github.com/drywaters/permitpal/internal/repository"
)

// bothRead holds each GetTracker call until two have been made, so two saves
// both read the stored hours before either writes them.
type bothRead struct {
	repository.Store
	arrived sync.WaitGroup
}

func (s *bothRead) GetTracker(ctx context.Context, driverID int64) (model.Tracker, error) {
	tracker, err := s.Store.GetTracker(ctx, driverID)
	s.arrived.Done()
	s.arrived.Wait()
	return tracker, err
}

func TestOverlappingProfileSavesCelebrateOnce(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			start := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
			store := &bothRead{Store: validationStore(t, backend, start)}
			driver, err := store.EnsureDriver(context.Background(), "overlap", start)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.UpdateProfile(context.Background(), driver.ID, model.Profile{TotalHours: 5}, start); err != nil {
				t.Fatal(err)
			}
			store.arrived.Add(2)
			var calls atomic.Int64
			h := NewDashboardHandler(store, time.UTC)
			h.now = func() time.Time { return start.Add(time.Duration(calls.Add(1)) * time.Minute) }
			bodies := make([]string, 2)
			var wg sync.WaitGroup
			for i := range bodies {
				wg.Add(1)
				go func() {
					defer wg.Done()
					form := url.Values{"total_hours": {"10"}, "night_hours": {"0"}, "permit_issue_date": {"2025-01-01"}}
					request := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
					request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					rec := httptest.NewRecorder()
					h.UpdateProfile(rec, request.WithContext(middleware.WithDriver(request.Context(), driver)))
					if rec.Code != http.StatusOK {
						t.Errorf("save %d: %d %s", i, rec.Code, rec.Body.String())
					}
					bodies[i] = rec.Body.String()
				}()
			}
			wg.Wait()
			celebrated := 0
			for _, body := range bodies {
				if strings.Contains(body, "milestone-banner") {
					celebrated++
				}
			}
			if celebrated != 1 {
				t.Fatalf("%d of two overlapping saves celebrated the 10 hour milestone, want 1", celebrated)
			}
		})
	}
}

func TestProfileSaveCelebratesOnlyTheSaveThatCrossesAMilestone(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			at := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
			store := validationStore(t, backend, at)
			driver, err := store.EnsureDriver(context.Background(), "milestones", at)
			if err != nil {
				t.Fatal(err)
			}
			h := NewDashboardHandler(store, time.UTC)
			h.now = func() time.Time { return at }
			submit := func(total, night string) string {
				t.Helper()
				form := url.Values{"total_hours": {total}, "night_hours": {night}, "permit_issue_date": {"2025-01-01"}}
				request := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				rec := httptest.NewRecorder()
				h.UpdateProfile(rec, request.WithContext(middleware.WithDriver(request.Context(), driver)))
				if rec.Code != http.StatusOK {
					t.Fatalf("save %s/%s: %d %s", total, night, rec.Code, rec.Body.String())
				}
				return rec.Body.String()
			}
			if body := submit("5", "0"); strings.Contains(body, "milestone-banner") {
				t.Fatalf("save below every line celebrated: %s", body)
			}
			at = at.AddDate(0, 0, 1)
			body := submit("12", "0")
			if !strings.Contains(body, "milestone-banner") || !strings.Contains(body, "10 total hours") || !strings.Contains(body, `datetime="2026-01-06"`) {
				t.Fatalf("crossing 10 hours was not celebrated and dated: %s", body)
			}
			at = at.Add(time.Minute)
			if strings.Contains(submit("12", "0"), "milestone-banner") {
				t.Fatal("resubmitting celebrated again")
			}
			// Dropping back below the line removes the badge; crossing again
			// celebrates again, with the new date.
			at = at.AddDate(0, 0, 1)
			if body := submit("8", "0"); strings.Contains(body, "milestone-banner") || strings.Contains(body, `datetime="2026-01-06"`) {
				t.Fatalf("correction kept the badge or celebrated: %s", body)
			}
			at = at.AddDate(0, 0, 1)
			if body := submit("10", "0"); !strings.Contains(body, "milestone-banner") || !strings.Contains(body, `datetime="2026-01-08"`) {
				t.Fatalf("re-crossing was not celebrated with the new date: %s", body)
			}
		})
	}
}
