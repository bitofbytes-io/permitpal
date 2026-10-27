package model

import (
	"math"
	"testing"
	"time"
)

func dateOn(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d }
func saveHours(profile Profile, total, night float64, at time.Time) Profile {
	next := Profile{PermitIssueDate: profile.PermitIssueDate, TotalHours: total, NightHours: night}
	next.HourEntries = AppendHourEntry(profile, next, at)
	return next
}

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

func TestWeeklyCreditChartBucketsAtWeekBoundaries(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// 23:30 Sunday in New York is already Monday in UTC. The recorded date stays
	// Sunday, and an hour later is Monday of the next ISO week.
	sunday := time.Date(2026, 3, 8, 23, 30, 0, 0, loc)
	monday := sunday.Add(time.Hour)
	if sunday.Weekday() != time.Sunday || monday.Weekday() != time.Monday || sunday.UTC().Day() == sunday.Day() {
		t.Fatalf("sunday=%s utc=%s monday=%s", sunday, sunday.UTC(), monday)
	}
	sameWeekSunday := time.Date(2026, 3, 15, 23, 30, 0, 0, loc)
	nextMonday := sameWeekSunday.Add(time.Hour)
	if sameWeekSunday.Weekday() != time.Sunday || nextMonday.Weekday() != time.Monday {
		t.Fatalf("boundary %s / %s", sameWeekSunday, nextMonday)
	}

	p := saveHours(Profile{}, 8, 1, sunday) // Week of Mar 2.
	p = saveHours(p, 11, 2, monday)         // Week of Mar 9 starts.
	p = saveHours(p, 24, 5, sameWeekSunday) // Same week, now over the cap.
	p = saveHours(p, 27, 6, nextMonday)     // Next Monday must not keep Sunday's hours.
	credit := CreditHours(p)
	chart := WeeklyCreditChart(credit, nextMonday, 12) // Window still contains Mar 2.
	assertChartMatchesCredit(t, credit, chart)

	sundayWeek := weekBar(t, chart, "2026-03-02")
	if sundayWeek.LoggedTotal != 8 || sundayWeek.CreditedTotal != 8 || sundayWeek.LoggedNight != 1 || sundayWeek.CreditedNight != 1 || sundayWeek.Capped {
		t.Fatalf("sunday week %+v", sundayWeek)
	}
	mondayWeek := weekBar(t, chart, "2026-03-09")
	if mondayWeek.LoggedTotal != 16 || mondayWeek.CreditedTotal != 10 || mondayWeek.LoggedNight != 4 || mondayWeek.CreditedNight != 4 || !mondayWeek.Capped {
		t.Fatalf("monday week %+v", mondayWeek)
	}
	following := weekBar(t, chart, "2026-03-16")
	if following.LoggedTotal != 3 || following.CreditedTotal != 3 || following.LoggedNight != 1 || following.CreditedNight != 1 || following.Capped {
		t.Fatalf("next monday %+v", following)
	}
	if chart.Weeks[0].Starts.Weekday() != time.Monday || !chart.Weeks[len(chart.Weeks)-1].Starts.Equal(isoWeekStart(nextMonday)) {
		t.Fatalf("window %+v", chart.Weeks)
	}

	// Move today forward until the Sunday week sits just outside the 12 bars
	// and the capped Monday week is the first bar.
	endMonday := dateOn("2026-03-09").AddDate(0, 0, 7*11)
	today := endMonday.AddDate(0, 0, 6)
	if endMonday.Weekday() != time.Monday || today.Weekday() != time.Sunday {
		t.Fatalf("end=%s today=%s", endMonday, today)
	}
	edge := WeeklyCreditChart(credit, today, 12)
	assertChartMatchesCredit(t, credit, edge)
	if !edge.Weeks[0].Starts.Equal(dateOn("2026-03-09")) || !edge.Weeks[0].Capped || edge.Weeks[0].CreditedTotal != 10 {
		t.Fatalf("first bar %+v", edge.Weeks[0])
	}
	if edge.EarlierTotal != 8 || edge.EarlierNight != 1 || edge.LaterTotal != 0 {
		t.Fatalf("outside window %+v", edge)
	}
	if edge.Weeks[1].LoggedTotal != 3 || edge.Weeks[2].LoggedTotal != 0 || edge.Weeks[2].Capped {
		t.Fatalf("later bars %+v %+v", edge.Weeks[1], edge.Weeks[2])
	}

	// The Sunday that closes the window stays in the last bar. The next Monday does not.
	closingAt := time.Date(today.Year(), today.Month(), today.Day(), 23, 30, 0, 0, loc)
	closing := saveHours(p, 29, 6, closingAt)
	closedCredit := CreditHours(closing)
	closed := WeeklyCreditChart(closedCredit, today, 12)
	assertChartMatchesCredit(t, closedCredit, closed)
	if last := closed.Weeks[len(closed.Weeks)-1]; !last.Starts.Equal(endMonday) || last.LoggedTotal != 2 || last.Capped {
		t.Fatalf("closing sunday %+v", last)
	}
	after := saveHours(closing, 33, 7, closingAt.Add(time.Hour))
	afterCredit := CreditHours(after)
	opened := WeeklyCreditChart(afterCredit, today, 12)
	assertChartMatchesCredit(t, afterCredit, opened)
	if last := opened.Weeks[len(opened.Weeks)-1]; last.LoggedTotal != 2 || last.CreditedNight != 0 {
		t.Fatalf("monday leaked into sunday week: %+v", last)
	}
	if opened.LaterTotal != 4 || opened.LaterNight != 1 {
		t.Fatalf("later week %+v", opened)
	}

	dash := NewDashboard(NewDriver("aiden"), after, nil, today)
	if dash.WeekChart.TotalHours != dash.HourCredit.TotalHours || dash.WeekChart.NightHours != dash.HourCredit.NightHours || !dash.WeekChart.Weeks[0].Capped {
		t.Fatalf("dashboard chart %+v credit %+v", dash.WeekChart, dash.HourCredit)
	}
}

func TestWeeklyCreditChartCapFlagsOnlyHoursOverTheCap(t *testing.T) {
	monday := dateOn("2026-03-09")
	exact := weekBar(t, WeeklyCreditChart(CreditHours(saveHours(Profile{}, 10, 4, monday)), monday, 12), "2026-03-09")
	if exact.Capped || exact.LoggedTotal != 10 || exact.CreditedTotal != 10 || exact.CreditedNight != 4 {
		t.Fatalf("exact cap %+v", exact)
	}
	over := weekBar(t, WeeklyCreditChart(CreditHours(saveHours(Profile{}, 10.1, 4, monday)), monday, 12), "2026-03-09")
	if !over.Capped || over.CreditedTotal != 10 || hourTenths(over.LoggedTotal) != 101 || over.CreditedNight != 4 {
		t.Fatalf("over cap %+v", over)
	}
}

func TestWeeklyCreditChartIncludesUndatedOpeningInTotal(t *testing.T) {
	monday := dateOn("2026-03-09")
	p := saveHours(Profile{TotalHours: 4, NightHours: 1}, 20, 6, monday)
	credit := CreditHours(p)
	chart := WeeklyCreditChart(credit, monday, 12)
	assertChartMatchesCredit(t, credit, chart)
	bar := weekBar(t, chart, "2026-03-09")
	if chart.UndatedTotal != 4 || chart.UndatedNight != 1 || !bar.Capped || bar.LoggedTotal != 16 || bar.CreditedTotal != 10 || bar.CreditedNight != 5 {
		t.Fatalf("chart %+v bar %+v", chart, bar)
	}
	legacy := CreditHours(Profile{TotalHours: 34.5, NightHours: 6})
	legacyChart := WeeklyCreditChart(legacy, monday, 12)
	assertChartMatchesCredit(t, legacy, legacyChart)
	if legacyChart.UndatedTotal != 34.5 || legacyChart.WindowTotal != 0 || legacyChart.Weeks[0].LoggedTotal != 0 {
		t.Fatalf("legacy %+v", legacyChart)
	}
}

func weekBar(t *testing.T, chart WeekCreditChart, start string) WeekCreditBar {
	t.Helper()
	day := dateOn(start)
	for _, week := range chart.Weeks {
		if week.Starts.Equal(day) {
			return week
		}
	}
	t.Fatalf("missing week %s", start)
	return WeekCreditBar{}
}

func assertChartMatchesCredit(t *testing.T, credit HourCredit, chart WeekCreditChart) {
	t.Helper()
	if chart.TotalHours != credit.TotalHours || chart.NightHours != credit.NightHours {
		t.Fatalf("chart totals %v/%v, credit %v/%v", chart.TotalHours, chart.NightHours, credit.TotalHours, credit.NightHours)
	}
	if len(chart.Weeks) != 12 {
		t.Fatalf("weeks=%d", len(chart.Weeks))
	}
	sumTotal := chart.WindowTotal + chart.EarlierTotal + chart.LaterTotal + chart.UndatedTotal
	sumNight := chart.WindowNight + chart.EarlierNight + chart.LaterNight + chart.UndatedNight
	if math.Round(sumTotal*10) != math.Round(credit.TotalHours*10) || math.Round(sumNight*10) != math.Round(credit.NightHours*10) {
		t.Fatalf("sum %v/%v credit %v/%v", sumTotal, sumNight, credit.TotalHours, credit.NightHours)
	}
	for i := 1; i < len(chart.Weeks); i++ {
		if !chart.Weeks[i].Starts.Equal(chart.Weeks[i-1].Starts.AddDate(0, 0, 7)) || chart.Weeks[i].Starts.Weekday() != time.Monday {
			t.Fatalf("week gap %+v", chart.Weeks[i])
		}
	}
}
