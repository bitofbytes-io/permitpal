package model

import (
	"testing"
	"time"
)

func TestEstimateReadyDateUsesAveragePaceFromStartDate(t *testing.T) {
	issueDate := time.Date(2025, 7, 24, 0, 0, 0, 0, time.Local)
	profile := Profile{
		PermitIssueDate: &issueDate,
		TotalHours:      34.5,
		NightHours:      6.0,
	}
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)

	got := estimateText(EstimateReadyDate(profile, now))
	if got != "On pace for November 25, 2026" {
		t.Fatalf("EstimateReadyDate() = %q, want %q", got, "On pace for November 25, 2026")
	}
}

func TestEstimateReadyDateRequiresNightHours(t *testing.T) {
	issueDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.Local)
	profile := Profile{
		PermitIssueDate: &issueDate,
		TotalHours:      60,
		NightHours:      0,
	}
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)

	got := estimateText(EstimateReadyDate(profile, now))
	if got != "Add night hours to estimate" {
		t.Fatalf("EstimateReadyDate() = %q, want %q", got, "Add night hours to estimate")
	}
}

func TestPercentClampsAtOneHundred(t *testing.T) {
	if got := Percent(75, 60); got != 100 {
		t.Fatalf("Percent() = %d, want 100", got)
	}
}

func TestPercentDoesNotRoundIncompleteProgressToOneHundred(t *testing.T) {
	if got := Percent(59.7, 60); got != 99 {
		t.Fatalf("Percent() = %d, want 99", got)
	}
}

func TestParseDateUsesGivenLocation(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseDate("2026-05-02", loc)
	if err != nil || got == nil {
		t.Fatalf("ParseDate returned %v, %v", got, err)
	}
	if got.Location() != loc || DateValue(got) != "2026-05-02" {
		t.Fatalf("ParseDate = %v, want 2026-05-02 in %v", got, loc)
	}
}

func TestEstimateUsesLocalDateLateInTheEvening(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// Postgres returns dates as UTC midnight; 23:30 in New York is already the next day in UTC.
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 26, 23, 30, 0, 0, loc)
	profile := Profile{PermitIssueDate: &start, TotalHours: 25, NightHours: 10}
	if got := estimateText(EstimateReadyDate(profile, now)); got != "On pace for October 31, 2026" {
		t.Fatalf("EstimateReadyDate() = %q, want pace from 25 elapsed days", got)
	}
}

func TestParseRatingRejectsUnknownValues(t *testing.T) {
	if _, ok := ParseRating("done"); ok {
		t.Fatal("ParseRating accepted unknown value")
	}
}

func TestNewDriverDefaultsAndFocus(t *testing.T) {
	now := time.Now()
	requirements := DefaultRequirements(now)
	want := []string{"Quick stop", "Turn about", "Stop on grade", "Start on grade", "Backing", "Approach corner", "Right turns", "Left turns", "Traffic lights", "Use of controls", "Starts", "Use of lane", "Use of brake", "Following", "Attention", "Stop signs", "Parking"}
	if len(requirements) != len(want) {
		t.Fatal("wrong checklist length")
	}
	for i, req := range requirements {
		if req.Title != want[i] || req.SortOrder != i+1 || req.Rating != RatingNotRated || req.RatedOn != nil || req.Description == "" {
			t.Fatalf("bad default: %+v", req)
		}
	}
	requirements[5].Rating = RatingFair
	requirements[9].Rating = RatingBad
	requirements[10].Rating = RatingBad
	requirements[0].Rating = RatingGood
	dash := NewDashboard(NewDriver("aiden"), NewDriverProfile(now), requirements, now)
	if dash.Driver.DisplayName != "Aiden" || dash.GoodCount != 1 || dash.PracticeFocus[0].SortOrder != 10 || dash.PracticeFocus[1].SortOrder != 11 || dash.PracticeFocus[2].SortOrder != 6 {
		t.Fatalf("dashboard=%+v", dash)
	}
	if dash.ReadyEstimate != (ReadyEstimate{Hint: "Add permit issue date to estimate"}) {
		t.Fatalf("%+v", dash.ReadyEstimate)
	}
	if rating, ok := ParseRating(""); !ok || rating != RatingNotRated {
		t.Fatal("empty rating should be not rated")
	}
}
func TestEstimateUsesLaterNightProjection(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start.AddDate(0, 0, 30)
	profile := Profile{PermitIssueDate: &start, TotalHours: 30, NightHours: 2}
	if got := estimateText(EstimateReadyDate(profile, now)); got != "On pace for May 31, 2026" {
		t.Fatal(got)
	}
	profile.TotalHours = 60
	profile.NightHours = 10
	if got := estimateText(EstimateReadyDate(profile, now)); got != "Ready when every skill is rated Good" {
		t.Fatal(got)
	}
}

// estimateText renders an estimate the way the dashboard shows it.
func estimateText(estimate ReadyEstimate) string {
	if estimate.Date != nil {
		return "On pace for " + estimate.Date.Format("January 2, 2006")
	}
	return estimate.Hint
}
