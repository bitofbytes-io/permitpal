package model

import (
	"testing"
)

func TestRecordedWeeklyCreditAndCorrections(t *testing.T) {
	p := saveHours(Profile{}, 15, 2, dateOn("2026-01-30"))
	check := func(total, night float64) {
		t.Helper()
		got := CreditHours(p)
		if got.TotalHours != total || got.NightHours != night {
			t.Fatalf("credit=%+v, want %v total/%v night", got, total, night)
		}
	}
	check(10, 2)
	p = saveHours(p, 15, 2, dateOn("2026-01-31"))
	if len(p.HourEntries) != 1 {
		t.Fatal("identical resubmission journaled")
	}
	p = saveHours(p, 12, 2, dateOn("2026-01-31"))
	check(10, 2)
	p = saveHours(p, 8, 2, dateOn("2026-02-01"))
	check(8, 2)
	p = saveHours(p, 13, 3, dateOn("2026-02-02"))
	check(13, 3)
	// A later-week reduction must remove newest matching lots, not only this
	// week's credit, or the old over-cap hours would incorrectly stay credited.
	p = saveHours(p, 6, 1, dateOn("2026-02-09"))
	check(6, 1)
	p = saveHours(p, 6, 3, dateOn("2026-02-10"))
	check(6, 3)
	if last := p.HourEntries[len(p.HourEntries)-1]; DateValue(&last.RecordedOn) != "2026-02-10" {
		t.Fatal(last)
	}
}

func TestRecordedTotalsBeyondRequirementsCanEarnLaterWeekCredit(t *testing.T) {
	first := dateOn("2026-01-05")
	p := saveHours(Profile{}, 60, 20, first)
	if c := CreditHours(p); c.TotalHours != 10 || c.NightHours != 10 {
		t.Fatal(c)
	}
	p = saveHours(p, 60, 20, first.AddDate(0, 0, 7))
	if len(p.HourEntries) != 1 {
		t.Fatal("unchanged submission added credit")
	}
	p = saveHours(p, 61, 21, first.AddDate(0, 0, 7))
	if c := CreditHours(p); c.TotalHours != 11 || c.NightHours != 11 {
		t.Fatalf("later credit=%+v", c)
	}
	// Keep recording new increments through enough weeks to meet both goals.
	for week := 2; week <= 6; week++ {
		p = saveHours(p, 61+10*float64(week-1), 21, first.AddDate(0, 0, 7*week))
	}
	if c := CreditHours(p); c.TotalHours != 61 || c.NightHours != 11 {
		t.Fatalf("goals remain unreachable: %+v", c)
	}
}
