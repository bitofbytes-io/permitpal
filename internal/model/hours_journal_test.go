package model

import (
	"testing"
	"time"
)

func dateOn(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d }
func saveHours(profile Profile, total, night float64, at time.Time) Profile {
	next := Profile{PermitIssueDate: profile.PermitIssueDate, TotalHours: total, NightHours: night}
	next.HourEntries = AppendHourEntry(profile, next, at)
	return next
}

func TestEntryDatesUseLocalISOWeeksAndSurviveTimezoneChanges(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	sunday := time.Date(2027, 1, 3, 23, 30, 0, 0, loc) // January 4 in UTC, ISO 2026-W53 locally.
	p := saveHours(Profile{}, 15, 1, sunday)
	p = saveHours(p, 25, 2, sunday.Add(time.Hour))
	credit := CreditHours(p)
	if credit.TotalHours != 20 || len(credit.Weeks) != 2 || !credit.Weeks[0].Starts.Equal(dateOn("2026-12-28")) || !credit.Weeks[1].Starts.Equal(dateOn("2027-01-04")) {
		t.Fatalf("credit=%+v", credit)
	}
	if !p.HourEntries[0].RecordedAt.Equal(sunday.UTC()) || !p.HourEntries[0].RecordedOn.Equal(dateOn("2027-01-03")) {
		t.Fatal("lost instant or recorded local date")
	}
}

func TestLegacyAndOldWriterChangesRemainUndated(t *testing.T) {
	p := Profile{TotalHours: 34.5, NightHours: 6}
	p = saveHours(p, 39.5, 7, dateOn("2026-10-21"))
	c := CreditHours(p)
	if c.TotalHours != 39.5 || c.UndatedTotal != 34.5 || len(c.Weeks) != 1 {
		t.Fatalf("credit=%+v", c)
	}
	// An old binary writes without journaling. Reconcile its change without
	// inferring a week from profile.updated_at.
	p.TotalHours = 42.5
	c = CreditHours(p)
	if c.UndatedTotal != 37.5 || c.TotalHours != 42.5 {
		t.Fatalf("old-writer credit=%+v", c)
	}
	p = saveHours(p, 43.5, 7, dateOn("2026-10-22"))
	if len(p.HourEntries) != 4 || !p.HourEntries[2].RecordedOn.IsZero() {
		t.Fatalf("entries=%+v", p.HourEntries)
	}
	// Reductions still reverse dated lots before an undated reconciliation.
	p = saveHours(p, 35.5, 7, dateOn("2026-10-23"))
	c = CreditHours(p)
	if c.UndatedTotal != 34.5 || c.TotalHours != 35.5 {
		t.Fatalf("correction=%+v", c)
	}
	invalid := CreditHours(Profile{TotalHours: 3, NightHours: 5, HourEntries: []HourEntry{{TotalTenths: 30, NightTenths: 50}}})
	if invalid.TotalHours != 3 || invalid.NightHours != 3 {
		t.Fatal(invalid)
	}
}
