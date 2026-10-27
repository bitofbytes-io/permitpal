package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/middleware"
	"github.com/drywaters/permitpal/internal/model"
	"github.com/go-chi/chi/v5"
)

func TestMasteredDateTransitions(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			now := time.Date(2026, 10, 27, 15, 4, 0, 0, time.Local)
			store := validationStore(t, backend, now)
			ctx := context.Background()
			driver, err := store.EnsureDriver(ctx, "aiden", now)
			if err != nil {
				t.Fatal(err)
			}
			handler := NewDashboardHandler(store, time.Local)
			handler.now = func() time.Time { return now }
			router := chi.NewRouter()
			router.Post("/requirements/{key}", handler.UpdateRequirement)
			submit := func(form url.Values) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/requirements/quick-stop", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req.WithContext(middleware.WithDriver(req.Context(), driver)))
				return rec
			}
			load := func() model.Requirement {
				t.Helper()
				tracker, err := store.GetTracker(ctx, driver.ID)
				if err != nil {
					t.Fatal(err)
				}
				req, ok := model.RequirementByKey(tracker.Requirements, "quick-stop")
				if !ok {
					t.Fatal("quick-stop missing")
				}
				return req
			}

			rec := submit(url.Values{"rating": {"good"}, "notes": {"Signed off"}})
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Mastered Oct 27, 2026") {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if got := load(); got.Rating != model.RatingGood || model.DateValue(got.RatedOn) != "2026-10-27" || got.Notes != "Signed off" {
				t.Fatalf("mastered save: %+v", got)
			}

			rec = submit(url.Values{"rating": {"good"}, "rated_on": {"2026-08-15"}, "notes": {"Edited"}})
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Mastered Aug 15, 2026") || strings.Contains(rec.Body.String(), "Last rated") {
				t.Fatalf("edited mastered status=%d body=%s", rec.Code, rec.Body.String())
			}
			if got := load(); model.DateValue(got.RatedOn) != "2026-08-15" || got.Notes != "Edited" {
				t.Fatalf("edited date: %+v", got)
			}

			// The same sign-off date submitted with a practice rating is cleared.
			rec = submit(url.Values{"rating": {"fair"}, "rated_on": {"2026-08-15"}, "notes": {"Needs practice"}})
			if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "Mastered") {
				t.Fatalf("practice status=%d body=%s", rec.Code, rec.Body.String())
			}
			if got := load(); got.Rating != model.RatingFair || got.RatedOn != nil || got.Notes != "Needs practice" {
				t.Fatalf("fair did not clear the mastered date: %+v", got)
			}

			rec = submit(url.Values{"rating": {"good"}, "rated_on": {"2026-08-15"}, "notes": {"Needs practice"}})
			if rec.Code != http.StatusOK {
				t.Fatal(rec.Body.String())
			}
			rec = submit(url.Values{"rating": {"bad"}, "notes": {"Needs practice"}})
			if rec.Code != http.StatusOK {
				t.Fatal(rec.Body.String())
			}
			if got := load(); got.Rating != model.RatingBad || got.RatedOn != nil {
				t.Fatalf("bad did not clear the mastered date: %+v", got)
			}

			rec = submit(url.Values{"rating": {"good"}, "rated_on": {"2026-08-15"}, "notes": {"Keep this note"}})
			if rec.Code != http.StatusOK {
				t.Fatal(rec.Body.String())
			}
			rec = submit(url.Values{"rating": {"good"}, "rated_on": {"2026-08-15"}, "notes": {"Keep this note"}, "clear_rating": {"true"}})
			if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "Mastered") {
				t.Fatalf("clear status=%d body=%s", rec.Code, rec.Body.String())
			}
			if got := load(); got.Rating != model.RatingNotRated || got.RatedOn != nil || got.Notes != "Keep this note" {
				t.Fatalf("clear rating: %+v", got)
			}
		})
	}
}
