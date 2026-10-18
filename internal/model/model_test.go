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
	// Slow enough that the pace, not the permit hold, sets the date.
	profile := Profile{PermitIssueDate: &start, TotalHours: 2, NightHours: 10}
	if got := estimateText(EstimateReadyDate(profile, now)); got != "On pace for September 20, 2028" {
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
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start.AddDate(0, 0, 300)
	profile := Profile{PermitIssueDate: &start, TotalHours: 30, NightHours: 2}
	estimate := EstimateReadyDate(profile, now)
	if got := estimateText(estimate); got != "On pace for February 9, 2029" || estimate.WaitingOn != ConstraintNightHours {
		t.Fatalf("EstimateReadyDate() = %q waiting on %q", got, estimate.WaitingOn)
	}
	profile.TotalHours = 60
	profile.NightHours = 10
	if got := estimateText(EstimateReadyDate(profile, now)); got != "Ready when every skill is rated Good" {
		t.Fatal(got)
	}
}

func TestEstimateWaitsOnTotalHoursWhenSlowest(t *testing.T) {
	issueDate := time.Date(2025, 7, 24, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	profile := Profile{PermitIssueDate: &issueDate, TotalHours: 34.5, NightHours: 6}
	if estimate := EstimateReadyDate(profile, now); estimate.WaitingOn != ConstraintTotalHours {
		t.Fatalf("WaitingOn = %q, want %q", estimate.WaitingOn, ConstraintTotalHours)
	}
}

func TestEstimateHoldsUntilNineMonthsAfterIssue(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		now          time.Time
		total, night float64
		want         string
	}{
		{"hours complete on issue date", time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC), 60, 10, "Eligible on October 1, 2026"},
		{"hours started on issue date", time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC), 60, 4, "Add hours to estimate"},
		{"pace ahead of hold", time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), 40, 10, "Eligible on October 1, 2026"},
		{"hours complete before hold", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 60, 10, "Eligible on October 1, 2026"},
		{"day before hold ends", time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC), 60, 10, "Eligible on October 1, 2026"},
		{"hold ends today", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), 60, 10, "Ready when every skill is rated Good"},
	}
	for _, tt := range tests {
		profile := Profile{PermitIssueDate: &start, TotalHours: tt.total, NightHours: tt.night}
		estimate := EstimateReadyDate(profile, tt.now)
		if got := estimateText(estimate); got != tt.want {
			t.Errorf("%s: EstimateReadyDate() = %q, want %q", tt.name, got, tt.want)
		}
		if estimate.Date != nil && estimate.WaitingOn != ConstraintPermitHold {
			t.Errorf("%s: WaitingOn = %q, want %q", tt.name, estimate.WaitingOn, ConstraintPermitHold)
		}
	}
}

func TestEstimateHoldClampsToMonthEnd(t *testing.T) {
	tests := []struct {
		start time.Time
		want  string
	}{
		// February 2027 has no 31st, so the hold ends on its last day.
		{time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), "Eligible on February 28, 2027"},
		{time.Date(2027, 5, 31, 0, 0, 0, 0, time.UTC), "Eligible on February 29, 2028"},
		{time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC), "Eligible on April 30, 2027"},
		{time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC), "Eligible on February 28, 2027"},
		{time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC), "Eligible on February 15, 2027"},
	}
	for _, tt := range tests {
		profile := Profile{PermitIssueDate: &tt.start, TotalHours: 60, NightHours: 10}
		if got := estimateText(EstimateReadyDate(profile, tt.start.AddDate(0, 0, 1))); got != tt.want {
			t.Errorf("issued %s: EstimateReadyDate() = %q, want %q", DateValue(&tt.start), got, tt.want)
		}
	}
}

// estimateText renders an estimate the way the dashboard shows it.
func estimateText(estimate ReadyEstimate) string {
	if estimate.Date != nil {
		label := "On pace for "
		if estimate.WaitingOn == ConstraintPermitHold {
			label = "Eligible on "
		}
		return label + estimate.Date.Format("January 2, 2006")
	}
	return estimate.Hint
}
