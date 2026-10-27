package model

import (
	"math"
	"sort"
	"time"
)

// HourEntry is a cumulative snapshot, not a drive. A zero RecordedOn marks
// an undated opening balance. Dated entries retain the server's local save
// date even if APP_TIMEZONE later changes.
type HourEntry struct {
	RecordedAt  time.Time
	RecordedOn  time.Time
	TotalTenths int
	NightTenths int
}

func hourTenths(hours float64) int { return int(math.Round(hours * 10)) }

// AppendHourEntry compares against the stored profile while its repository
// lock is held. Identical submissions and permit-only changes add no hours.
func AppendHourEntry(previous, next Profile, at time.Time) []HourEntry {
	entries := ReconciledHourEntries(previous)

	if hourTenths(previous.TotalHours) != hourTenths(next.TotalHours) || hourTenths(previous.NightHours) != hourTenths(next.NightHours) {
		entries = append(entries, HourEntry{RecordedAt: at, RecordedOn: startOfDay(at), TotalTenths: hourTenths(next.TotalHours), NightTenths: hourTenths(next.NightHours)})
	}
	return entries
}

type hourLot struct {
	date       time.Time
	day, night int
}
type WeeklyHours struct {
	Starts                 time.Time
	TotalHours, NightHours float64
	RawTotal, RawNight     float64
}
type HourCredit struct {
	TotalHours, NightHours     float64
	UndatedTotal, UndatedNight float64
	Weeks                      []WeeklyHours
	LastRecordedOn             *time.Time
}

// LastDriveHighlightAfterDays is the longest quiet gap. The dashboard
// highlights the nudge only once more than this many calendar days have
// passed since the latest dated hour entry.
const LastDriveHighlightAfterDays = 7

// LastDrive is the days since the latest dated hour entry and the credit
// still open in today's Monday–Sunday week. Dated is false for an empty
// journal and for undated opening hours. RemainingHours is WeeklyHourCap
// minus this week's already-credited total from HourCredit.
type LastDrive struct {
	Dated          bool
	Days           int
	RecordedOn     *time.Time
	RemainingHours float64
	Stale          bool
}

// DaysSinceLastDrive measures the calendar-day gap from today back to
// credit.LastRecordedOn. A gap of LastDriveHighlightAfterDays stays quiet;
// a longer gap is Stale. An entry dated after today counts as zero days and
// is not stale. Remaining credit reuses the capped weekly totals: other
// weeks and undated hours do not spend this week's cap.
func DaysSinceLastDrive(credit HourCredit, today time.Time) LastDrive {
	drive := LastDrive{RemainingHours: remainingWeekCredit(credit, today)}
	if credit.LastRecordedOn == nil || credit.LastRecordedOn.IsZero() {
		return drive
	}
	on := startOfDay(*credit.LastRecordedOn)
	days := int(startOfDay(today).Sub(on).Hours() / 24)
	if days < 0 {
		days = 0
	}
	drive.Dated = true
	drive.Days = days
	drive.RecordedOn = &on
	drive.Stale = days > LastDriveHighlightAfterDays
	return drive
}

// remainingWeekCredit is the cap minus the credited total HourCredit already
// assigned to today's ISO week. It does not replay hour lots.
func remainingWeekCredit(credit HourCredit, today time.Time) float64 {
	week := isoWeekStart(today)
	creditedTenths := 0
	for _, entry := range credit.Weeks {
		if entry.Starts.Equal(week) {
			creditedTenths = hourTenths(entry.TotalHours)
			break
		}
	}
	remaining := WeeklyHourCap*10 - creditedTenths
	if remaining < 0 {
		return 0
	}
	return float64(remaining) / 10
}

// CreditHours replays day and night independently. A correction removes the
// newest matching hours first, then the undated opening balance. Reclassifying
// day to night therefore records the newly classified night hours today.
// Each ISO Monday–Sunday week grants at most 10 total hours, choosing night
// hours first so credited night is always a subset of credited total.
func CreditHours(profile Profile) HourCredit {
	entries := ReconciledHourEntries(profile)
	if len(entries) == 0 { // Old profiles supplied without a journal remain unverified.
		return HourCredit{TotalHours: profile.TotalHours, NightHours: profile.NightHours, UndatedTotal: profile.TotalHours, UndatedNight: profile.NightHours}
	}
	lots := []hourLot{}
	previousDay, previousNight := 0, 0
	credit := HourCredit{}
	for _, entry := range entries {
		night := min(entry.TotalTenths, entry.NightTenths)
		day := entry.TotalTenths - night
		dayDelta, nightDelta := day-previousDay, night-previousNight
		// Reverse latest dated lots before the opening balance, regardless of
		// snapshot order. Within each bucket reductions cannot create new credit.
		for _, undated := range []bool{false, true} {
			for i := len(lots) - 1; i >= 0 && (dayDelta < 0 || nightDelta < 0); i-- {
				if lots[i].date.IsZero() != undated {
					continue
				}
				if dayDelta < 0 {
					take := min(lots[i].day, -dayDelta)
					lots[i].day -= take
					dayDelta += take
				}
				if nightDelta < 0 {
					take := min(lots[i].night, -nightDelta)
					lots[i].night -= take
					nightDelta += take
				}
			}
		}
		if dayDelta > 0 || nightDelta > 0 {
			lots = append(lots, hourLot{entry.RecordedOn, max(0, dayDelta), max(0, nightDelta)})
		}
		previousDay, previousNight = day, night
		if !entry.RecordedOn.IsZero() {
			d := entry.RecordedOn
			credit.LastRecordedOn = &d
		}
	}
	weeks := map[time.Time]hourLot{}
	for _, lot := range lots {
		if lot.date.IsZero() {
			credit.UndatedTotal += float64(lot.day+lot.night) / 10
			credit.UndatedNight += float64(lot.night) / 10
			continue
		}
		week := isoWeekStart(lot.date)
		sum := weeks[week]
		sum.day += lot.day
		sum.night += lot.night
		weeks[week] = sum
	}
	credit.TotalHours, credit.NightHours = credit.UndatedTotal, credit.UndatedNight
	for date, lot := range weeks {
		total := min(WeeklyHourCap*10, lot.day+lot.night)
		night := min(total, lot.night)
		credit.TotalHours += float64(total) / 10
		credit.NightHours += float64(night) / 10
		credit.Weeks = append(credit.Weeks, WeeklyHours{date, float64(total) / 10, float64(night) / 10, float64(lot.day+lot.night) / 10, float64(lot.night) / 10})
	}
	sort.Slice(credit.Weeks, func(i, j int) bool { return credit.Weeks[i].Starts.Before(credit.Weeks[j].Starts) })
	return credit
}

// WeekCreditBar is one Monday–Sunday entry week: hours logged that week and
// the hours credited after the weekly cap. Capped weeks logged more than the cap.
type WeekCreditBar struct {
	Starts                       time.Time
	LoggedTotal, LoggedNight     float64
	CreditedTotal, CreditedNight float64
	Capped                       bool
}

// WeekCreditChart is the recent entry weeks ending at today. Credited hours in
// the window, earlier weeks, later weeks, and the undated opening balance add
// up to TotalHours and NightHours, which are the HourCredit totals shown on
// the dashboard.
type WeekCreditChart struct {
	Weeks                      []WeekCreditBar
	WindowTotal, WindowNight   float64
	EarlierTotal, EarlierNight float64
	LaterTotal, LaterNight     float64
	UndatedTotal, UndatedNight float64
	TotalHours, NightHours     float64
}

// WeeklyCreditChart places credited and logged hours into the last `weeks`
// ISO weeks ending at today. A Sunday stays in the week that began the
// previous Monday, so a Sunday night and the following Monday land in
// adjacent bars. Hours outside the window stay in Earlier or Later and still
// count toward the dashboard totals.
func WeeklyCreditChart(credit HourCredit, today time.Time, weeks int) WeekCreditChart {
	if weeks < 1 {
		weeks = ChartWeeks
	}
	end := isoWeekStart(today)
	start := end.AddDate(0, 0, -7*(weeks-1))
	byWeek := make(map[time.Time]WeeklyHours, len(credit.Weeks))
	for _, week := range credit.Weeks {
		byWeek[week.Starts] = week
	}
	chart := WeekCreditChart{
		Weeks:        make([]WeekCreditBar, 0, weeks),
		UndatedTotal: credit.UndatedTotal,
		UndatedNight: credit.UndatedNight,
		TotalHours:   credit.TotalHours,
		NightHours:   credit.NightHours,
	}
	inWindow := make(map[time.Time]bool, weeks)
	for i := 0; i < weeks; i++ {
		day := start.AddDate(0, 0, 7*i)
		inWindow[day] = true
		bar := WeekCreditBar{Starts: day}
		if week, ok := byWeek[day]; ok {
			bar.LoggedTotal = week.RawTotal
			bar.LoggedNight = week.RawNight
			bar.CreditedTotal = week.TotalHours
			bar.CreditedNight = week.NightHours
			bar.Capped = hourTenths(week.RawTotal) > WeeklyHourCap*10
		}
		chart.WindowTotal += bar.CreditedTotal
		chart.WindowNight += bar.CreditedNight
		chart.Weeks = append(chart.Weeks, bar)
	}
	for _, week := range credit.Weeks {
		if inWindow[week.Starts] {
			continue
		}
		if week.Starts.Before(start) {
			chart.EarlierTotal += week.TotalHours
			chart.EarlierNight += week.NightHours
			continue
		}
		chart.LaterTotal += week.TotalHours
		chart.LaterNight += week.NightHours
	}
	return chart
}

func isoWeekStart(date time.Time) time.Time {
	date = startOfDay(date)
	return date.AddDate(0, 0, -(int(date.Weekday())+6)%7)
}

// ReconciledHourEntries treats writes from an older binary during rollout as
// undated changes. It never invents a submission date for those writes.
func ReconciledHourEntries(profile Profile) []HourEntry {
	entries := append([]HourEntry(nil), profile.HourEntries...)
	total, night := hourTenths(profile.TotalHours), hourTenths(profile.NightHours)
	if len(entries) == 0 {
		if total != 0 || night != 0 {
			entries = append(entries, HourEntry{TotalTenths: total, NightTenths: night})
		}
	} else if last := entries[len(entries)-1]; last.TotalTenths != total || last.NightTenths != night {
		entries = append(entries, HourEntry{TotalTenths: total, NightTenths: night})
	}
	return entries
}

// projectedCreditDays evaluates any forecast day in constant time, then finds
// the first qualifying day by bisection. Complete weeks are summed directly,
// so slow observed paces do not cause an unbounded daily simulation.
func projectedCreditDays(credit HourCredit, today time.Time, totalPace, nightPace, totalTarget, nightTarget float64) int {
	week := isoWeekStart(today)
	currentTotal, currentNight := 0.0, 0.0
	for _, w := range credit.Weeks {
		if w.Starts.Equal(week) {
			currentTotal, currentNight = w.RawTotal, w.RawNight
		}
	}
	weekCredit := func(total, night float64) (float64, float64) {
		total = math.Min(WeeklyHourCap, total)
		return total, math.Min(total, night)
	}
	initialTotal, initialNight := weekCredit(currentTotal, currentNight)
	remaining := 6 - (int(today.Weekday())+6)%7
	complete := func(days int) bool {
		currentDays := min(days, remaining)
		total, night := weekCredit(currentTotal+float64(currentDays)*totalPace, currentNight+float64(currentDays)*nightPace)
		total += credit.TotalHours - initialTotal
		night += credit.NightHours - initialNight
		future := max(0, days-remaining)
		fullTotal, fullNight := weekCredit(7*totalPace, 7*nightPace)
		total += float64(future/7) * fullTotal
		night += float64(future/7) * fullNight
		tailTotal, tailNight := weekCredit(float64(future%7)*totalPace, float64(future%7)*nightPace)
		return total+tailTotal+1e-9 >= totalTarget && night+tailNight+1e-9 >= nightTarget
	}
	if complete(0) {
		return 0
	}
	high := 1
	for !complete(high) {
		high *= 2
	}
	low := 0
	for low+1 < high {
		mid := low + (high-low)/2
		if complete(mid) {
			high = mid
		} else {
			low = mid
		}
	}
	return high
}
