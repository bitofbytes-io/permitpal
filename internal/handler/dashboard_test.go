package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/repository"
	"github.com/go-chi/chi/v5"
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
		"status": {"needs_practice"},
		"notes":  {notes},
	}
	req := httptest.NewRequest(http.MethodPost, "/requirements/lane-changes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router := chi.NewRouter()
	handler := NewDashboardHandler(repository.NewMemoryStore(time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)))
	router.Post("/requirements/{key}", handler.UpdateRequirement)

	router.ServeHTTP(rec, req)

	return rec
}
