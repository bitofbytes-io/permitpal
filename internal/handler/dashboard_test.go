package handler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/middleware"
	"github.com/drywaters/permitpal/internal/model"
	"github.com/drywaters/permitpal/internal/repository"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func TestParseHoursAllowsSingleDecimalPlace(t *testing.T) {
	tests := map[string]float64{
		"":     0,
		"0":    0,
		"6":    6,
		"6.0":  6,
		"34.5": 34.5,
		".5":   0.5,
	}

	for value, want := range tests {
		got, err := parseHours(value, 60)
		if err != nil {
			t.Fatalf("parseHours(%q) returned error: %v", value, err)
		}
		if got != want {
			t.Fatalf("parseHours(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestParseHoursRejectsMoreThanOneDecimalPlace(t *testing.T) {
	for _, value := range []string{"1.23", "34.50", "6.01", "1e1", "-1"} {
		if _, err := parseHours(value, 60); err == nil {
			t.Fatalf("parseHours(%q) returned nil error", value)
		}
	}
}

func TestParseHoursRejectsValuesAboveMaximum(t *testing.T) {
	for _, value := range []string{"60.1", "99999"} {
		if _, err := parseHours(value, 60); err == nil {
			t.Fatalf("parseHours(%q) returned nil error", value)
		}
	}
}

func TestUpdateRequirementReturnsSavedFeedback(t *testing.T) {
	rec := updateRequirementWithNotes(t, "practiced smooth merges")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Saved") {
		t.Fatalf("body = %q, want Saved feedback", rec.Body.String())
	}
}

func TestUpdateProfileReturnsProgressSavedFeedback(t *testing.T) {
	form := url.Values{
		"total_hours":       {"12.5"},
		"night_hours":       {"2.0"},
		"permit_issue_date": {"2026-01-15"},
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	store := repository.NewMemoryStore(time.Now())
	driver, err := store.EnsureDriver(context.Background(), "aiden", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req = req.WithContext(middleware.WithDriver(req.Context(), driver))
	handler := NewDashboardHandler(store, time.Local)

	handler.UpdateProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Progress saved") {
		t.Fatalf("body = %q, want Progress saved feedback", rec.Body.String())
	}
}

func TestRequirementNotesLengthLimit(t *testing.T) {
	notes := strings.Repeat("a", maxNotesChars+1)
	rec := updateRequirementWithNotes(t, notes)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	want := "Notes must be 1000 characters or fewer"
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("body = %q, want %q", rec.Body.String(), want)
	}
}

func TestRequirementNotesLengthLimitCountsRunes(t *testing.T) {
	notes := strings.Repeat("é", maxNotesChars)

	rec := updateRequirementWithNotes(t, notes)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %q", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func updateRequirementWithNotes(t *testing.T, notes string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{
		"rating": {"fair"},
		"notes":  {notes},
	}
	req := httptest.NewRequest(http.MethodPost, "/requirements/use-of-lane", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router := chi.NewRouter()
	store := repository.NewMemoryStore(time.Now())
	driver, err := store.EnsureDriver(context.Background(), "aiden", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req = req.WithContext(middleware.WithDriver(req.Context(), driver))
	handler := NewDashboardHandler(store, time.Local)
	router.Post("/requirements/{key}", handler.UpdateRequirement)

	router.ServeHTTP(rec, req)

	return rec
}

func TestRatingDefaultsToLocalDateLateInTheEvening(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 23, 30, 0, 0, loc)
	store := repository.NewMemoryStore(now)
	driver, err := store.EnsureDriver(context.Background(), "aiden", now)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewDashboardHandler(store, loc)
	handler.now = func() time.Time { return now.UTC() }
	router := chi.NewRouter()
	router.Post("/requirements/{key}", handler.UpdateRequirement)
	form := url.Values{"rating": {"good"}}
	req := httptest.NewRequest(http.MethodPost, "/requirements/quick-stop", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(middleware.WithDriver(req.Context(), driver)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %q", rec.Code, rec.Body.String())
	}
	tracker, err := store.GetTracker(context.Background(), driver.ID)
	if err != nil {
		t.Fatal(err)
	}
	requirement, _ := model.RequirementByKey(tracker.Requirements, "quick-stop")
	if got := model.DateValue(requirement.RatedOn); got != "2026-09-26" {
		t.Fatalf("rated on = %q, want the New York date 2026-09-26", got)
	}
}

type failingTrackerStore struct{ repository.Store }

func (failingTrackerStore) GetTracker(context.Context, int64) (model.Tracker, error) {
	return model.Tracker{}, errors.New("database unavailable")
}

func TestServerErrorsLogTheCauseAndRequestID(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	handler := chimw.RequestID(http.HandlerFunc(NewDashboardHandler(failingTrackerStore{}, time.UTC).Dashboard))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(chimw.RequestIDHeader, "req-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req.WithContext(middleware.WithDriver(req.Context(), model.Driver{ID: 1})))

	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "database unavailable") {
		t.Fatalf("status=%d body=%q; want a 500 that hides the cause", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"level=ERROR", `error="database unavailable"`, "request_id=req-123", "path=/"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("log %q is missing %s", logs.String(), want)
		}
	}
}
