package model

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	TotalHoursRequired = 60.0
	NightHoursRequired = 10.0
	// MinPermitHoldMonths is how long the permit must be held before the
	// road test, counted from the issue date.
	MinPermitHoldMonths = 9
	// WeeklyHourCap is the maximum credited driving hours per calendar week.
	// TODO(BOB-177): apply this cap once dated per-drive logging exists;
	// aggregate profile hours cannot establish weekly credited totals.
	WeeklyHourCap = 10
)

// North Carolina jurisdiction copy used by the road-test checklist.
const (
	level2LicenseAgeDetail = "The minimum age for a Level 2 license."
	dmvInsuranceDetail     = "Bring proof of liability insurance to the DMV."
)

type RequirementRating string

const (
	RatingNotRated RequirementRating = "not_rated"
	RatingGood     RequirementRating = "good"
	RatingBad      RequirementRating = "bad"
	RatingFair     RequirementRating = "fair"
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
}

type Requirement struct {
	Key         string
	Title       string
	Description string
	Rating      RequirementRating
	RatedOn     *time.Time
	Notes       string
	SortOrder   int
	UpdatedAt   time.Time
}

// Tracker is one driver's stored progress: hours, permit date and skill ratings.
type Tracker struct {
	Profile      Profile
	Requirements []Requirement
}

type Dashboard struct {
	Driver        Driver
	Profile       Profile
	Requirements  []Requirement
	PracticeFocus []Requirement
	ReadyEstimate ReadyEstimate
	PermitHold    PermitHold
	TotalPercent  int
	NightPercent  int
	GoodCount     int
}

func NewDashboard(driver Driver, profile Profile, requirements []Requirement, now time.Time) Dashboard {
	ordered := append([]Requirement(nil), requirements...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].SortOrder < ordered[j].SortOrder })
	good := 0
	for _, req := range ordered {
		if req.Rating == RatingGood {
			good++
		}
	}
	focus := make([]Requirement, 0, 3)
	for _, rating := range []RequirementRating{RatingBad, RatingFair, RatingNotRated} {
		for _, req := range ordered {
			if req.Rating == rating && len(focus) < 3 {
				focus = append(focus, req)
			}
		}
	}
	return Dashboard{Driver: driver, Profile: profile, Requirements: ordered, PracticeFocus: focus, ReadyEstimate: EstimateReadyDate(profile, now), PermitHold: NewPermitHold(profile, now), TotalPercent: Percent(profile.TotalHours, TotalHoursRequired), NightPercent: Percent(profile.NightHours, NightHoursRequired), GoodCount: good}
}

func Percent(value, target float64) int {
	if target <= 0 {
		return 0
	}
	if value >= target {
		return 100
	}
	return int(math.Max(0, math.Min(99, math.Floor(value/target*100))))
}

// ReadyConstraint names the requirement that sets a ready estimate's date.
type ReadyConstraint string

const (
	ConstraintTotalHours ReadyConstraint = "total_hours"
	ConstraintNightHours ReadyConstraint = "night_hours"
	ConstraintPermitHold ReadyConstraint = "permit_hold"
)

// ReadyEstimate is the projected date the hour requirements will be met and
// the permit hold will have passed, with the constraint that sets it, or,
// when there is none, a hint about what the driver should do next.
type ReadyEstimate struct {
	Date      *time.Time
	WaitingOn ReadyConstraint
	Hint      string
}

// EstimateReadyDate projects manually entered aggregate hours and applies the
// permit-hold floor. It does not calculate weekly credited hours.
func EstimateReadyDate(profile Profile, now time.Time) ReadyEstimate {
	if profile.PermitIssueDate == nil {
		return ReadyEstimate{Hint: "Add permit issue date to estimate"}
	}
	startDate := startOfDay(*profile.PermitIssueDate)
	today := startOfDay(now)
	// No days have passed to set a pace, but completed hours need none: the
	// permit hold alone then sets the date.
	hoursComplete := profile.TotalHours >= TotalHoursRequired && profile.NightHours >= NightHoursRequired
	if !today.After(startDate) && !hoursComplete {
		return ReadyEstimate{Hint: "Add hours to estimate"}
	}

	readyDate, ok := projectedRequirementDate(startDate, today, profile.TotalHours, TotalHoursRequired)
	if !ok {
		return ReadyEstimate{Hint: "Add hours to estimate"}
	}

	nightDate, ok := projectedRequirementDate(startDate, today, profile.NightHours, NightHoursRequired)
	if !ok {
		return ReadyEstimate{Hint: "Add night hours to estimate"}
	}
	waitingOn := ConstraintTotalHours
	if nightDate.After(readyDate) {
		readyDate, waitingOn = nightDate, ConstraintNightHours
	}
	if holdEnd := permitHoldEnd(startDate); holdEnd.After(readyDate) {
		readyDate, waitingOn = holdEnd, ConstraintPermitHold
	}
	if !readyDate.After(today) {
		return ReadyEstimate{Hint: "Ready when every skill is rated Good"}
	}
	return ReadyEstimate{Date: &readyDate, WaitingOn: waitingOn}
}

// PermitHold counts down to the day the permit hold ends. Ends is nil until
// the permit issue date is set; DaysLeft is 0 once the hold is over.
type PermitHold struct {
	Ends     *time.Time
	DaysLeft int
}

func NewPermitHold(profile Profile, now time.Time) PermitHold {
	if profile.PermitIssueDate == nil {
		return PermitHold{}
	}
	ends := permitHoldEnd(startOfDay(*profile.PermitIssueDate))
	days := int(math.Round(ends.Sub(startOfDay(now)).Hours() / 24))
	return PermitHold{Ends: &ends, DaysLeft: max(days, 0)}
}

// RoadTestStep is a road-test requirement that the hour counts and skill
// ratings do not track.
type RoadTestStep struct {
	Title  string
	Detail string
}

// RoadTestSteps lists what the driver needs besides hours, skills and the
// permit hold. While the hold runs, the road-test step gives the earliest
// test date without saying when appointments open, which can change.
func RoadTestSteps(hold PermitHold) []RoadTestStep {
	roadTest := "Book a road test appointment."
	if hold.Ends != nil && hold.DaysLeft > 0 {
		roadTest = "The test date must be " + DisplayDate(hold.Ends) + " or later."
	}
	return []RoadTestStep{
		{"Be at least 16", level2LicenseAgeDetail},
		{"6 months with no violations", "No moving, seat belt, or cell phone convictions."},
		{"Proof of insurance", dmvInsuranceDetail},
		{"Schedule the road test", roadTest},
	}
}

func FormatHours(hours float64) string {
	return fmt.Sprintf("%.1f", hours)
}

func ParseRating(value string) (RequirementRating, bool) {
	switch RequirementRating(value) {
	case "", RatingNotRated:
		return RatingNotRated, true
	case RatingBad, RatingFair, RatingGood:
		return RequirementRating(value), true
	default:
		return "", false
	}
}

func NewDriverProfile(now time.Time) Profile { return Profile{UpdatedAt: now} }

func DefaultRequirements(now time.Time) []Requirement {
	items := []struct{ key, title, description string }{
		{"quick-stop", "Quick stop", "Stop quickly and safely on the examiner's signal, keeping the car straight and under control."},
		{"turn-about", "Turn about", "Check traffic, signal, and complete the turn safely with control and room to maneuver."},
		{"stop-on-grade", "Stop on grade", "Stop under control, turn the wheels correctly for the slope, and set the parking brake."},
		{"start-on-grade", "Start on grade", "Check traffic, release the parking brake, and move smoothly without rolling backward."},
		{"backing", "Backing", "Look behind you and back slowly in a straight line while checking for traffic."},
		{"approach-corner", "Approach corner", "Check traffic, signal early, and slow to a safe speed before reaching the corner."},
		{"right-turns", "Right turns", "Signal, yield to pedestrians and traffic, and turn into the proper lane without swinging wide."},
		{"left-turns", "Left turns", "Signal, yield to oncoming traffic and pedestrians, and finish in the proper lane."},
		{"traffic-lights", "Traffic lights", "Observe signals, stop behind the line, and check traffic before proceeding on green."},
		{"use-of-controls", "Use of controls", "Use mirrors, signals, steering, and other vehicle controls correctly without distraction."},
		{"starts", "Starts", "Check mirrors and blind spots, signal when needed, and accelerate smoothly into traffic."},
		{"use-of-lane", "Use of lane", "Keep a steady lane position and check mirrors and blind spots before changing lanes."},
		{"use-of-brake", "Use of brake", "Brake smoothly and early enough to stop safely while keeping control."},
		{"following", "Following", "Maintain at least a 2–3 second gap and allow more space in poor conditions."},
		{"attention", "Attention", "Watch the road, scan for hazards, and respond safely to traffic and pedestrians."},
		{"stop-signs", "Stop signs", "Make a complete stop behind the line and check traffic before proceeding."},
		{"parking", "Parking", "Check surroundings, signal, and park safely within the space with the vehicle secured."},
	}
	requirements := make([]Requirement, len(items))
	for i, item := range items {
		requirements[i] = Requirement{Key: item.key, Title: item.title, Description: item.description, Rating: RatingNotRated, SortOrder: i + 1, UpdatedAt: now}
	}
	return requirements
}

func RequirementByKey(requirements []Requirement, key string) (Requirement, bool) {
	for _, req := range requirements {
		if req.Key == key {
			return req, true
		}
	}
	return Requirement{}, false
}

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

func projectedRequirementDate(startDate, today time.Time, current, required float64) (time.Time, bool) {
	if current >= required {
		return today, true
	}
	if current <= 0 {
		return time.Time{}, false
	}
	elapsedDays := today.Sub(startDate).Hours() / 24
	if elapsedDays <= 0 {
		return time.Time{}, false
	}
	daysToRequired := int(math.Ceil(required / current * elapsedDays))
	return startDate.AddDate(0, 0, daysToRequired), true
}
