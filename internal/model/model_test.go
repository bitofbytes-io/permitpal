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

func TestPercentDoesNotRoundIncompleteProgressToOneHundred(t *testing.T) {
	if got := Percent(59.7, 60); got != 99 {
		t.Fatalf("Percent() = %d, want 99", got)
	}
}

func TestNormalizeDateUsesLocalLocation(t *testing.T) {
	got := NormalizeDate("2026-05-02")
	if got == nil {
		t.Fatal("NormalizeDate returned nil")
	}
	if got.Location() != time.Local {
		t.Fatalf("location = %v, want %v", got.Location(), time.Local)
	}
}

func TestParseStatusRejectsUnknownValues(t *testing.T) {
	if _, ok := ParseStatus("done"); ok {
		t.Fatal("ParseStatus accepted unknown value")
	}
}
