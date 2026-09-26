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

type Dashboard struct {
	Driver        Driver
	Profile       Profile
	Requirements  []Requirement
	PracticeFocus []Requirement
	ReadyEstimate string
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
	return Dashboard{Driver: driver, Profile: profile, Requirements: ordered, PracticeFocus: focus, ReadyEstimate: EstimateReadyDate(profile, now), TotalPercent: Percent(profile.TotalHours, TotalHoursRequired), NightPercent: Percent(profile.NightHours, NightHoursRequired), GoodCount: good}
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

func EstimateReadyDate(profile Profile, now time.Time) string {
	if profile.PermitIssueDate == nil {
		return "Add permit issue date to estimate"
	}
	startDate := startOfDay(*profile.PermitIssueDate)
	today := startOfDay(now)
	if !today.After(startDate) {
		return "Add hours to estimate"
	}

	readyDate, ok := projectedRequirementDate(startDate, today, profile.TotalHours, TotalHoursRequired)
	if !ok {
		return "Add hours to estimate"
	}

	nightDate, ok := projectedRequirementDate(startDate, today, profile.NightHours, NightHoursRequired)
	if !ok {
		return "Add night hours to estimate"
	}
	if nightDate.After(readyDate) {
		readyDate = nightDate
	}
	if !readyDate.After(today) {
		return "Ready when every skill is rated Good"
	}
	return "On pace for " + readyDate.Format("January 2, 2006")
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

func ParseDate(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation("2006-01-02", value, time.Local)
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

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
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
