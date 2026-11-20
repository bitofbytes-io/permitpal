package model

import (
	"testing"
	"time"
)

func TestWeeklyCapPushesFastEstimateBeyondMonthEnd(t *testing.T) {
	start := dateOn("2025-01-01")                                         // Hold is already complete.
	p := Profile{PermitIssueDate: &start, TotalHours: 50, NightHours: 10} // Undated opening.
	p = saveHours(p, 59, 10, dateOn("2026-01-31"))                        // Saturday, 9 credited this week.
	// Raw pace is slow after a long permit hold. Force a deliberately recent
	// observation start to test the future weekly cap itself independently.
	c := CreditHours(p)
	got := projectedCreditDays(c, dateOn("2026-01-31"), 15, 1, 60, 10)
	if got != 1 {
		t.Fatalf("one remaining hour fits Sunday: %d", got)
	}
	// Replace opening with a full 60-hour same-week entry: only 10 are credited.
	p = saveHours(Profile{PermitIssueDate: &start}, 60, 10, dateOn("2026-01-31"))
	estimate := EstimateReadyDate(p, dateOn("2026-01-31"))
	if estimate.WaitingOn != ConstraintWeeklyCap || estimate.Date == nil || !estimate.Date.After(dateOn("2026-02-28")) {
		t.Fatalf("estimate=%+v", estimate)
	}
	// High pace, exhausted current week: each next Monday starts new capacity.
	if days := projectedCreditDays(CreditHours(p), dateOn("2026-01-31"), 20, 5, 60, 10); days != 30 {
		t.Fatalf("cap finish days=%d, want March 2 (30)", days)
	}
}

func TestWeeklyCapMonthEndAndHoldFloor(t *testing.T) {
	start := dateOn("2026-05-31")
	p := saveHours(Profile{PermitIssueDate: &start}, 60, 10, dateOn("2027-02-27"))
	estimate := EstimateReadyDate(p, dateOn("2027-02-27"))
	if estimate.Date == nil || !estimate.Date.After(dateOn("2027-02-28")) || estimate.WaitingOn != ConstraintWeeklyCap {
		t.Fatalf("estimate=%+v", estimate)
	}
	// A future hold wins when it is strictly later than capped hours completion.
	start = dateOn("2027-02-01")
	p.PermitIssueDate = &start
	estimate = EstimateReadyDate(p, dateOn("2027-02-27"))
	if DateValue(estimate.Date) != "2027-11-01" || estimate.WaitingOn != ConstraintPermitHold {
		t.Fatalf("estimate=%+v", estimate)
	}
}

func TestProjectionRecomputesNightAcrossWholeWeek(t *testing.T) {
	p := saveHours(Profile{}, 10, 0, dateOn("2026-02-02"))
	// Tuesday's projected night replaces daytime within Monday's full cap.
	if days := projectedCreditDays(CreditHours(p), dateOn("2026-02-02"), 2, 2, 10, 2); days != 1 {
		t.Fatalf("late-week night requires %d days", days)
	}
	// Extremely slow pace forecasts by week arithmetic, not daily iteration.
	if days := projectedCreditDays(HourCredit{}, dateOn("2026-02-02"), 0.000001, 0.000001, 60, 10); days != 60000000 {
		t.Fatalf("slow forecast=%d", days)
	}
}

func TestDaysSinceLastDrive(t *testing.T) {
	today := dateOn("2026-10-27") // Tuesday. This ISO week starts Monday Oct 26.

	none := DaysSinceLastDrive(CreditHours(Profile{}), today)
	if none.Dated || none.Days != 0 || none.Stale || none.RecordedOn != nil || none.RemainingHours != WeeklyHourCap {
		t.Fatalf("empty journal %+v", none)
	}

	opening := DaysSinceLastDrive(CreditHours(Profile{TotalHours: 34.5, NightHours: 6}), today)
	if opening.Dated || opening.Stale || opening.RemainingHours != WeeklyHourCap {
		t.Fatalf("undated opening hours %+v", opening)
	}

	// Seven days is still quiet. The entry falls in the previous week, so
	// this week's credit is untouched.
	quietDay := dateOn("2026-10-20")
	quiet := DaysSinceLastDrive(CreditHours(saveHours(Profile{}, 4, 1, quietDay)), today)
	if !quiet.Dated || quiet.Days != 7 || quiet.Stale || quiet.RemainingHours != WeeklyHourCap || DateValue(quiet.RecordedOn) != "2026-10-20" {
		t.Fatalf("seven-day gap %+v", quiet)
	}

	eight := DaysSinceLastDrive(CreditHours(saveHours(Profile{}, 1, 0, dateOn("2026-10-19"))), today)
	if !eight.Dated || eight.Days != 8 || !eight.Stale || eight.RemainingHours != WeeklyHourCap {
		t.Fatalf("eight-day gap %+v", eight)
	}

	staleDay := dateOn("2026-10-13")
	staleProfile := saveHours(Profile{TotalHours: 20, NightHours: 3}, 26.5, 5, staleDay)
	stale := DaysSinceLastDrive(CreditHours(staleProfile), today)
	if !stale.Dated || stale.Days != 14 || !stale.Stale || stale.RemainingHours != WeeklyHourCap || DateValue(stale.RecordedOn) != "2026-10-13" {
		t.Fatalf("stale gap with undated opening %+v", stale)
	}

	// Hours saved today spend this week's credit. A capped week leaves none.
	logged := DaysSinceLastDrive(CreditHours(saveHours(Profile{}, 6.5, 2, today)), today)
	if logged.Days != 0 || logged.Stale || logged.RemainingHours != 3.5 {
		t.Fatalf("same-day credit %+v", logged)
	}
	capped := DaysSinceLastDrive(CreditHours(saveHours(Profile{}, 12, 4, today)), today)
	if capped.RemainingHours != 0 || capped.Stale {
		t.Fatalf("capped week %+v", capped)
	}

	// A later dated entry wins. An undated reconciliation after it does not
	// erase the date or move those hours into this week.
	journal := saveHours(saveHours(Profile{}, 3, 1, staleDay), 8, 2, today)
	journal.TotalHours = 11
	later := DaysSinceLastDrive(CreditHours(journal), today)
	// Five hours were dated today. The extra three are an undated reconciliation
	// and must not spend this week's credit or replace today's entry date.
	if later.Days != 0 || later.Stale || DateValue(later.RecordedOn) != "2026-10-27" || later.RemainingHours != 5 {
		t.Fatalf("latest dated entry %+v", later)
	}

	// Spring-forward night still counts calendar dates, and Sunday's hours
	// stay in the week that began the previous Monday.
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	sunday := time.Date(2026, 3, 8, 23, 30, 0, 0, loc)
	monday := time.Date(2026, 3, 9, 0, 30, 0, 0, loc)
	week := DaysSinceLastDrive(CreditHours(saveHours(Profile{}, 2, 0, sunday)), monday)
	if week.Days != 1 || week.Stale || week.RemainingHours != WeeklyHourCap || DateValue(week.RecordedOn) != "2026-03-08" {
		t.Fatalf("dst week boundary %+v", week)
	}

	// Remaining credit uses the capped weekly total, not a fresh hour replay.
	weekStart := dateOn("2026-10-26")
	built := HourCredit{
		LastRecordedOn: &today,
		Weeks:          []WeeklyHours{{Starts: weekStart, TotalHours: 10, RawTotal: 16, NightHours: 4, RawNight: 4}},
	}
	if got := DaysSinceLastDrive(built, today); got.RemainingHours != 0 || got.Days != 0 || got.Stale {
		t.Fatalf("built credit %+v", got)
	}
	otherWeek := dateOn("2026-10-19")
	built.Weeks = []WeeklyHours{{Starts: otherWeek, TotalHours: 10, RawTotal: 16}}
	if got := DaysSinceLastDrive(built, today); got.RemainingHours != WeeklyHourCap {
		t.Fatalf("other week spent this week: %+v", got)
	}
	built.Weeks = []WeeklyHours{{Starts: weekStart, TotalHours: 9.9}}
	if got := DaysSinceLastDrive(built, today); got.RemainingHours != 0.1 {
		t.Fatalf("tenth of an hour %+v", got)
	}

	future := dateOn("2026-10-28")
	ahead := DaysSinceLastDrive(HourCredit{LastRecordedOn: &future}, today)
	if ahead.Days != 0 || ahead.Stale || !ahead.Dated || ahead.RemainingHours != WeeklyHourCap {
		t.Fatalf("future entry %+v", ahead)
	}

	dash := NewDashboard(NewDriver("aiden"), staleProfile, nil, today)
	if !dash.LastDrive.Dated || dash.LastDrive.Days != stale.Days || dash.LastDrive.Stale != stale.Stale || dash.LastDrive.RemainingHours != stale.RemainingHours || DateValue(dash.LastDrive.RecordedOn) != DateValue(stale.RecordedOn) {
		t.Fatalf("dashboard last drive %+v, want %+v", dash.LastDrive, stale)
	}
}
