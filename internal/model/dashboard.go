package model

import (
	"math"
	"sort"
	"time"
)

// North Carolina jurisdiction copy used by the road-test checklist.
const (
	level2LicenseAgeDetail = "The minimum age for a Level 2 license."
	dmvInsuranceDetail     = "Bring proof of liability insurance to the DMV."
)

type Dashboard struct {
	Driver        Driver
	Profile       Profile
	Requirements  []Requirement
	PracticeFocus []Requirement
	ReadyEstimate ReadyEstimate
	PermitHold    PermitHold
	Readiness     RoadTestReadiness
	TotalPercent  int
	NightPercent  int
	GoodCount     int
	HourCredit    HourCredit
	WeekChart     WeekCreditChart
	LastDrive     LastDrive
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
	credit := CreditHours(profile)
	return Dashboard{
		Driver:        driver,
		Profile:       profile,
		Requirements:  ordered,
		PracticeFocus: focus,
		ReadyEstimate: EstimateReadyDate(profile, now),
		PermitHold:    NewPermitHold(profile, now),
		TotalPercent:  Percent(profile.TotalHours, TotalHoursRequired),
		NightPercent:  Percent(profile.NightHours, NightHoursRequired),
		GoodCount:     good,
		HourCredit:    credit,
		WeekChart:     WeeklyCreditChart(credit, now, ChartWeeks),
		LastDrive:     DaysSinceLastDrive(credit, now),
		Readiness:     NewRoadTestReadiness(credit, ordered),
	}
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
	ConstraintWeeklyCap  ReadyConstraint = "weekly_cap"
)

// ReadyEstimate is the projected date the hour requirements will be met and
// the permit hold will have passed, with the constraint that sets it, or,
// when there is none, a hint about what the driver should do next.
type ReadyEstimate struct {
	Date      *time.Time
	WaitingOn ReadyConstraint
	Hint      string
}

// EstimateReadyDate uses the observed cumulative pace and entry-week credit,
// then applies the permit-hold floor. Entry dates do not verify drive dates.
func EstimateReadyDate(profile Profile, now time.Time) ReadyEstimate {
	if profile.PermitIssueDate == nil {
		return ReadyEstimate{Hint: "Add permit issue date to estimate"}
	}
	start, today := startOfDay(*profile.PermitIssueDate), startOfDay(now)
	credit := CreditHours(profile)
	readyDate := today
	waitingOn := ConstraintTotalHours
	if credit.TotalHours < TotalHoursRequired || credit.NightHours < NightHoursRequired {
		elapsed := today.Sub(start).Hours() / 24
		if elapsed <= 0 || profile.TotalHours <= 0 {
			return ReadyEstimate{Hint: "Add hours to estimate"}
		}
		if profile.NightHours <= 0 {
			return ReadyEstimate{Hint: "Add night hours to estimate"}
		}
		totalPace, nightPace := profile.TotalHours/elapsed, profile.NightHours/elapsed
		totalDays := projectedCreditDays(credit, today, totalPace, nightPace, TotalHoursRequired, 0)
		nightDays := projectedCreditDays(credit, today, totalPace, nightPace, 0, NightHoursRequired)
		days := max(totalDays, nightDays)
		if nightDays > totalDays {
			waitingOn = ConstraintNightHours
		}
		uncappedDays := max(0, int(math.Ceil((TotalHoursRequired-profile.TotalHours)/totalPace)), int(math.Ceil((NightHoursRequired-profile.NightHours)/nightPace)))
		if days > uncappedDays {
			waitingOn = ConstraintWeeklyCap
		}
		readyDate = today.AddDate(0, 0, days)
	}
	if holdEnd := permitHoldEnd(start); holdEnd.After(readyDate) {
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
