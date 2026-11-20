package model

import (
	"math"
	"testing"
	"time"
)

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
