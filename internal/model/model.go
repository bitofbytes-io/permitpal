package model

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	TotalHoursRequired = 60.0
	NightHoursRequired = 10.0
)

type RequirementStatus string

const (
	StatusNeedsPractice RequirementStatus = "needs_practice"
	StatusMastered      RequirementStatus = "mastered"
)

type Profile struct {
	PermitIssueDate *time.Time
	TotalHours      float64
	NightHours      float64
	UpdatedAt       time.Time
}

type Requirement struct {
	Key          string
	Title        string
	Description  string
	Status       RequirementStatus
	MasteredDate *time.Time
	Notes        string
	SortOrder    int
	UpdatedAt    time.Time
}

type Dashboard struct {
	Profile       Profile
	Requirements  []Requirement
	PracticeFocus []Requirement
	ReadyEstimate string
	TotalPercent  int
	NightPercent  int
	MasteredCount int
}

func NewDashboard(profile Profile, requirements []Requirement, now time.Time) Dashboard {
	mastered := 0
	focus := make([]Requirement, 0, 3)
	for _, req := range requirements {
		if req.Status == StatusMastered {
			mastered++
			continue
		}
		if len(focus) < 3 {
			focus = append(focus, req)
		}
	}

	return Dashboard{
		Profile:       profile,
		Requirements:  requirements,
		PracticeFocus: focus,
		ReadyEstimate: EstimateReadyDate(profile, now),
		TotalPercent:  Percent(profile.TotalHours, TotalHoursRequired),
		NightPercent:  Percent(profile.NightHours, NightHoursRequired),
		MasteredCount: mastered,
	}
}

func Percent(value, target float64) int {
	if target <= 0 {
		return 0
	}
	return int(math.Max(0, math.Min(100, math.Round(value/target*100))))
}

func EstimateReadyDate(profile Profile, now time.Time) string {
	startDate := DrivingStartDate(profile)
	today := startOfDay(now)
	if !today.After(startDate) {
		return "Add hours to estimate"
	}

	readyDate, ok := projectedRequirementDate(startDate, today, profile.TotalHours, TotalHoursRequired)
	if !ok {
		return "Add hours to estimate"
	}
	if nightDate, ok := projectedRequirementDate(startDate, today, profile.NightHours, NightHoursRequired); ok && nightDate.After(readyDate) {
		readyDate = nightDate
	} else if profile.NightHours < NightHoursRequired && !ok {
		return "Add hours to estimate"
	}

	holdDate := startDate.AddDate(0, 9, 0)
	if holdDate.After(readyDate) {
		readyDate = holdDate
	}

	if !readyDate.After(today) {
		return "Ready when checklist is mastered"
	}
	return "On pace for " + readyDate.Format("January 2")
}

func FormatHours(hours float64) string {
	return fmt.Sprintf("%.1f", hours)
}

func ParseStatus(value string) RequirementStatus {
	switch RequirementStatus(value) {
	case StatusMastered:
		return StatusMastered
	default:
		return StatusNeedsPractice
	}
}

func DefaultProfile(now time.Time) Profile {
	issueDate := DefaultDrivingStartDate()
	return Profile{
		PermitIssueDate: &issueDate,
		TotalHours:      34.5,
		NightHours:      6.0,
		UpdatedAt:       now,
	}
}

func DefaultDrivingStartDate() time.Time {
	return time.Date(2025, time.July, 24, 0, 0, 0, 0, time.Local)
}

func DrivingStartDate(profile Profile) time.Time {
	if profile.PermitIssueDate != nil {
		return startOfDay(*profile.PermitIssueDate)
	}
	return DefaultDrivingStartDate()
}

func DefaultRequirements(now time.Time) []Requirement {
	masteredDate := func(month time.Month, day int) *time.Time {
		t := time.Date(2026, month, day, 0, 0, 0, 0, time.Local)
		return &t
	}
	items := []Requirement{
		{"starting-the-car", "Starting the car", "Adjust seat, buckle seat belt, adjust mirrors, start smoothly.", StatusMastered, masteredDate(time.March, 8), "Smooth startup routine.", 1, now},
		{"posture", "Posture", "Sit at least 10 inches from the wheel with clear visibility.", StatusMastered, masteredDate(time.March, 11), "Check mirrors before moving.", 2, now},
		{"forward-movement", "Forward movement", "Signal before pulling into traffic, accelerate smoothly, hold lane position.", StatusMastered, masteredDate(time.March, 18), "", 3, now},
		{"traffic-lights", "Traffic lights", "Observe signals, stop smoothly behind the line, start promptly on green.", StatusMastered, masteredDate(time.March, 25), "", 4, now},
		{"stop-signs", "Stop signs", "Observe signs early, stop completely, check traffic before proceeding.", StatusMastered, masteredDate(time.April, 2), "", 5, now},
		{"yield-caution-lights", "Yield signs and caution lights", "Adjust speed, check traffic flow, yield to vehicles with right of way.", StatusNeedsPractice, nil, "Needs calmer speed adjustment.", 6, now},
		{"lane-changes", "Lane changes", "Signal in advance, check mirrors, look over shoulder, change smoothly.", StatusNeedsPractice, nil, "Practice on multilane roads.", 7, now},
		{"turn-lanes", "Use of turn lanes", "Signal in advance, check mirrors and shoulder, enter turn lane properly.", StatusNeedsPractice, nil, "", 8, now},
		{"left-right-turns", "Left and right turns", "Signal, check traffic, leave space, stay in correct lane, avoid cutting corners.", StatusNeedsPractice, nil, "Good right turns; left turns need consistency.", 9, now},
		{"backing", "Backing", "Look over shoulder through rear window, back slowly and smoothly in a straight line.", StatusNeedsPractice, nil, "Practice driveway backing.", 10, now},
		{"parking", "Parking", "Signal into space, park between lines, pull completely into space.", StatusMastered, masteredDate(time.April, 12), "", 11, now},
		{"three-point-turn", "Three point turn / turn about", "Signal, stop, check mirrors and shoulders, complete turn safely without rushing.", StatusNeedsPractice, nil, "Next practice focus.", 12, now},
		{"driver-courtesy", "Driver courtesy", "Show patience, maintain distance, avoid aggressive driving.", StatusMastered, masteredDate(time.April, 20), "Calm and steady.", 13, now},
	}
	return items
}

func RequirementByKey(requirements []Requirement, key string) (Requirement, bool) {
	for _, req := range requirements {
		if req.Key == key {
			return req, true
		}
	}
	return Requirement{}, false
}

func NormalizeDate(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	return &t
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
