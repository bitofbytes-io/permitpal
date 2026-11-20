package model

import (
	"math"
	"time"
)

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
