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
	"github.com/drywaters/permitpal/internal/repository"
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

func TestUpdateProfileReturnsProgressSavedFeedback(t *testing.T) {
	form := url.Values{
		"total_hours":       {"12.5"},
		"night_hours":       {"2.0"},
		"permit_issue_date": {"2026-01-15"},
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	// Pin the clock after the submitted permit issue date.
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	store := repository.NewMemoryStore(at)
	driver, err := store.EnsureDriver(context.Background(), "aiden", at)
	if err != nil {
		t.Fatal(err)
	}
	req = req.WithContext(middleware.WithDriver(req.Context(), driver))
	handler := NewDashboardHandler(store, time.UTC)
	handler.now = func() time.Time { return at }

	handler.UpdateProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Progress saved") {
		t.Fatalf("body = %q, want Progress saved feedback", rec.Body.String())
	}
	// The countdown follows the saved permit issue date.
	if !strings.Contains(rec.Body.String(), `id="whats-left"`) || !strings.Contains(rec.Body.String(), "Oct 15, 2026") {
		t.Fatalf("body = %q, want the refreshed hold countdown", rec.Body.String())
	}
	// 12.5 recorded in one week credits 10, so the card uses that credit
	// rather than 60 minus the raw total.
	body := rec.Body.String()
	if !strings.Contains(body, `id="road-test-readiness"`) || !strings.Contains(body, `hx-swap-oob="outerHTML"`) || !strings.Contains(body, "50.0 total hours left") || !strings.Contains(body, "8.0 night hours left") {
		t.Fatalf("body = %q, want credited hours still left", body)
	}
}

func TestProfileSaveUsesOneServerLocalEntryTimestamp(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	at := time.Date(2027, 1, 3, 23, 30, 0, 0, loc)
	store := repository.NewMemoryStore(at)
	ctx := context.Background()
	driver, err := store.EnsureDriver(ctx, "aiden", at)
	if err != nil {
		t.Fatal(err)
	}
	h := NewDashboardHandler(store, loc)
	calls := 0
	h.now = func() time.Time { calls++; return at.UTC() }
	form := url.Values{"total_hours": {"15"}, "night_hours": {"10"}, "permit_issue_date": {"2025-01-01"}, "recorded_on": {"2027-01-04"}}
	request := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	h.UpdateProfile(response, request.WithContext(middleware.WithDriver(request.Context(), driver)))
	if response.Code != http.StatusOK || calls != 1 {
		t.Fatalf("status=%d, clock calls=%d", response.Code, calls)
	}
	tracker, err := store.GetTracker(ctx, driver.ID)
	if err != nil {
		t.Fatal(err)
	}
	if entries := tracker.Profile.HourEntries; len(entries) != 1 || !entries[0].RecordedAt.Equal(at) || model.DateValue(&entries[0].RecordedOn) != "2027-01-03" {
		t.Fatalf("entries=%+v", entries)
	}
	body := response.Body.String()
	for _, want := range []string{
		"Waiting on 10-hour weekly cap",
		"Jan 3, 2027",
		"Hours by entry week",
		"These 12 weeks credit 10.0 total and 10.0 night, the same weekly credited total shown above.",
		"Capped: week of Dec 28, 2026 (lost 5.0)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q: %s", want, body)
		}
	}
}

func TestCumulativeHoursCanPassCreditedRequirements(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			at := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
			store := validationStore(t, backend, at)
			ctx := context.Background()
			driver, err := store.EnsureDriver(ctx, "more-hours", at)
			if err != nil {
				t.Fatal(err)
			}
			h := NewDashboardHandler(store, time.UTC)
			h.now = func() time.Time { return at }
			submit := func(total, night string) *httptest.ResponseRecorder {
				form := url.Values{"total_hours": {total}, "night_hours": {night}, "permit_issue_date": {"2025-01-01"}}
				request := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				rec := httptest.NewRecorder()
				h.UpdateProfile(rec, request.WithContext(middleware.WithDriver(request.Context(), driver)))
				return rec
			}
			if rec := submit("60", "20"); rec.Code != http.StatusOK {
				t.Fatalf("first save: %d %s", rec.Code, rec.Body.String())
			}
			at = at.AddDate(0, 0, 7)
			if rec := submit("61", "21"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "11.0 total, including 11.0 night") {
				t.Fatalf("next-week save: %d %s", rec.Code, rec.Body.String())
			}
			if rec := submit("1000", "1000"); rec.Code != http.StatusOK {
				t.Fatalf("maximum save: %d %s", rec.Code, rec.Body.String())
			}
			for _, bad := range []string{"1000.1", "1001", "1e100", "NaN", "Inf"} {
				if rec := submit(bad, "21"); rec.Code != http.StatusBadRequest {
					t.Fatalf("total %q status=%d", bad, rec.Code)
				}
				if rec := submit("1000", bad); rec.Code != http.StatusBadRequest {
					t.Fatalf("night %q status=%d", bad, rec.Code)
				}
			}
			tracker, err := store.GetTracker(ctx, driver.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tracker.Profile.TotalHours != 1000 || tracker.Profile.NightHours != 1000 || len(tracker.Profile.HourEntries) != 3 {
				t.Fatalf("invalid save changed progress: %+v", tracker.Profile)
			}
		})
	}
}
