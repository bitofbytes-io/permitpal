package handler

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
	"github.com/drywaters/permitpal/internal/repository"
)

const quickStop = "/requirements/quick-stop"

func TestRequirementValidation(t *testing.T) {
	forEachBackend(t, func(t *testing.T, backend string) {
		// "now" is 2026-05-01 in the app time zone.
		f := newFormFixture(t, backend, time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local))
		for _, date := range []string{"not-a-date", "2026-02-30", "2026-13-01", "2026-01-15T12:00:00Z"} {
			for _, status := range []string{"good", "fair"} {
				t.Run("invalid-date/"+status+"/"+date, func(t *testing.T) {
					form := url.Values{"rating": {status}, "notes": {"must not be saved"}, "rated_on": {date}}
					f.requireRejected(t, quickStop, form, "Last rated date")
				})
			}
		}
		t.Run("future-rated-on-date", func(t *testing.T) {
			form := url.Values{"rating": {"good"}, "rated_on": {"2026-05-02"}}
			f.requireRejected(t, quickStop, form, "Last rated date cannot be in the future")
		})
		t.Run("clearing-a-saved-future-date", func(t *testing.T) {
			// A rating saved before future dates were rejected can still be cleared.
			req, _ := model.RequirementByKey(f.load(t).Requirements, "turn-about")
			future := f.now.AddDate(0, 0, 3)
			req.Rating, req.RatedOn = model.RatingGood, &future
			if _, err := f.store.UpdateRequirement(context.Background(), f.driver.ID, req); err != nil {
				t.Fatal(err)
			}
			f.requireOK(t, "/requirements/turn-about", url.Values{"rating": {"good"}, "rated_on": {future.Format("2006-01-02")}, "clear_rating": {"true"}})
			if cleared, _ := model.RequirementByKey(f.load(t).Requirements, "turn-about"); cleared.Rating != model.RatingNotRated || cleared.RatedOn != nil {
				t.Fatalf("requirement not cleared: %+v", cleared)
			}
		})
		t.Run("today-is-allowed", func(t *testing.T) {
			f.requireOK(t, quickStop, url.Values{"rating": {"good"}, "rated_on": {"2026-05-01"}})
			requirement, ok := model.RequirementByKey(f.load(t).Requirements, "quick-stop")
			if !ok || requirement.Rating != model.RatingGood || model.DateValue(requirement.RatedOn) != "2026-05-01" {
				t.Fatalf("today's rating was not saved: %+v", requirement)
			}
		})
		for _, date := range []string{"2026-01-15", "", "  "} {
			t.Run("valid-or-cleared-date/"+date, func(t *testing.T) {
				f.requireOK(t, quickStop, url.Values{"rating": {"good"}, "notes": {"updated"}, "rated_on": {date}})
				requirement, ok := model.RequirementByKey(f.load(t).Requirements, "quick-stop")
				if !ok || requirement.Rating != model.RatingGood || requirement.Notes != "updated" || model.DateValue(requirement.RatedOn) != expectedRatedDate(date, f.now) {
					t.Fatalf("unexpected saved requirement: %+v", requirement)
				}
			})
		}
	})
}

func expectedRatedDate(date string, now time.Time) string {
	if strings.TrimSpace(date) == "" {
		return now.Format("2006-01-02")
	}
	return strings.TrimSpace(date)
}

func TestRatingValuesAndDriverIsolation(t *testing.T) {
	forEachBackend(t, func(t *testing.T, backend string) {
		ctx := context.Background()
		now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
		f := newFormFixture(t, backend, now)
		caleb, err := f.store.EnsureDriver(ctx, "caleb", now)
		if err != nil {
			t.Fatal(err)
		}
		before := trackerFor(t, f.store, caleb)
		for _, rating := range []string{"mastered", "needs_practice", "great", "BAD"} {
			if rec := f.submit(t, quickStop, url.Values{"rating": {rating}}); rec.Code != http.StatusBadRequest {
				t.Fatalf("accepted %q: %d", rating, rec.Code)
			}
		}
		for _, rating := range []string{"bad", "fair", "good", "not_rated", ""} {
			rec := f.submit(t, quickStop, url.Values{"rating": {rating}, "notes": {"Aiden only"}})
			if rec.Code != http.StatusOK {
				t.Fatalf("save %q: %d %s", rating, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `id="practice-focus"`) || !strings.Contains(rec.Body.String(), `hx-swap-oob="outerHTML"`) {
				t.Fatal("response did not update practice focus out of band")
			}
			ratedOn := f.load(t).Requirements[0].RatedOn
			if rating == "" || rating == "not_rated" {
				if ratedOn != nil {
					t.Fatal("not rated kept date")
				}
			} else if model.DateValue(ratedOn) != "2026-05-01" {
				t.Fatal("rating missing today's date")
			}
		}
		f.requireOK(t, "/profile", url.Values{"total_hours": {"20"}, "night_hours": {"3"}})
		after := trackerFor(t, f.store, caleb)
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

func trackerFor(t *testing.T, store repository.Store, driver model.Driver) model.Tracker {
	t.Helper()
	tracker, err := store.GetTracker(context.Background(), driver.ID)
	if err != nil {
		t.Fatal(err)
	}
	return tracker
}

func TestClearRatingPreservesNotesAndUpdatesFocus(t *testing.T) {
	forEachBackend(t, func(t *testing.T, backend string) {
		// Pin "now" after the submitted rated_on date.
		f := newFormFixture(t, backend, time.Date(2026, 5, 2, 0, 0, 0, 0, time.Local))
		form := url.Values{"rating": {"good"}, "rated_on": {"2026-05-01"}, "notes": {"Keep this practice note"}}
		f.requireOK(t, quickStop, form)
		form.Set("clear_rating", "true")
		rec := f.submit(t, quickStop, form)
		if rec.Code != http.StatusOK {
			t.Fatal(rec.Body.String())
		}
		tracker := f.load(t)
		req := tracker.Requirements[0]
		if req.Rating != model.RatingNotRated || req.RatedOn != nil || req.Notes != "Keep this practice note" {
			t.Fatalf("cleared requirement=%+v", req)
		}
		if focus := model.NewDashboard(f.driver, tracker.Profile, tracker.Requirements, f.now).PracticeFocus; len(focus) == 0 || focus[0].Key != "quick-stop" {
			t.Fatal("clear did not restore practice focus")
		}
		if strings.Contains(rec.Body.String(), `checked`) || !strings.Contains(rec.Body.String(), `hx-swap-oob="outerHTML"`) || !strings.Contains(rec.Body.String(), "Clear rating for Quick stop") {
			t.Fatal("clear response did not reset selection and refresh focus")
		}
	})
}
