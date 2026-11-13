package model

import (
	"fmt"
	"strings"
	"time"
)

const (
	TotalHoursRequired = 60.0
	NightHoursRequired = 10.0
	// MaxRecordedHours bounds cumulative input independently of credited
	// requirements, leaving room to replace hours excluded by a weekly cap.
	// 1,000 hours fits numeric(5,1) and integer-tenths journal arithmetic.
	MaxRecordedHours = 1000
	// MinPermitHoldMonths is how long the permit must be held before the
	// road test, counted from the issue date.
	MinPermitHoldMonths = 9
	// WeeklyHourCap is the maximum credited driving hours per calendar week.
	// Applied to increments grouped by their recorded ISO calendar week.
	WeeklyHourCap = 10
	// ChartWeeks is how many recent Monday–Sunday entry weeks the dashboard chart shows.
	ChartWeeks = 12
)

type Driver struct {
	ID          int64
	Username    string
	DisplayName string
	// SessionGeneration is the generation a session cookie must carry;
	// logout increments it.
	SessionGeneration int64
}

func NewDriver(username string) Driver {
	name := username
	if len(name) > 0 {
		name = strings.ToUpper(name[:1]) + name[1:]
	}
	return Driver{Username: username, DisplayName: name}
}

type Profile struct {
	PermitIssueDate *time.Time
	TotalHours      float64
	NightHours      float64
	UpdatedAt       time.Time
	HourEntries     []HourEntry
}

// Tracker is one driver's stored progress: hours, permit date and skill ratings.
type Tracker struct {
	Profile      Profile
	Requirements []Requirement
}

func FormatHours(hours float64) string {
	return fmt.Sprintf("%.1f", hours)
}

func NewDriverProfile(now time.Time) Profile { return Profile{UpdatedAt: now} }

func ParseDate(value string, loc *time.Location) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation("2006-01-02", value, loc)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func DateValue(date *time.Time) string {
	if date == nil {
		return ""
	}
	return date.Format("2006-01-02")
}

func DisplayDate(date *time.Time) string {
	if date == nil {
		return "Not set"
	}
	return date.Format("Jan 2, 2006")
}

// startOfDay returns t's calendar date, taken in t's own location, as UTC
// midnight. Dates from Postgres (UTC) and the app time zone then compare as
// whole days, unaffected by offsets or DST.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// addMonths moves a UTC-midnight date forward by calendar months, clamping
// to the target month's last day when it is shorter (May 31 + 9 months is
// February 28) the way calendar months are usually counted. time.AddDate
// would roll over into March instead.
func addMonths(date time.Time, months int) time.Time {
	y, m, d := date.Date()
	firstOfTarget := time.Date(y, m+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	lastDay := firstOfTarget.AddDate(0, 1, -1).Day()
	return firstOfTarget.AddDate(0, 0, min(d, lastDay)-1)
}

// permitHoldEnd is the day the MinPermitHoldMonths hold ends, counted from a
// UTC-midnight issue date.
func permitHoldEnd(issueDate time.Time) time.Time {
	return addMonths(issueDate, MinPermitHoldMonths)
}
