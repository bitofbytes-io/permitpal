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

	got := EstimateReadyDate(profile, now)
	if got != "On pace for November 25" {
		t.Fatalf("EstimateReadyDate() = %q, want %q", got, "On pace for November 25")
	}
}

func TestEstimateReadyDateUsesPermitHoldWhenLater(t *testing.T) {
	issueDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.Local)
	profile := Profile{
		PermitIssueDate: &issueDate,
		TotalHours:      60,
		NightHours:      10,
	}
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)

	got := EstimateReadyDate(profile, now)
	if got != "On pace for October 15" {
		t.Fatalf("EstimateReadyDate() = %q, want %q", got, "On pace for October 15")
	}
}

func TestPercentClampsAtOneHundred(t *testing.T) {
	if got := Percent(75, 60); got != 100 {
		t.Fatalf("Percent() = %d, want 100", got)
	}
}
