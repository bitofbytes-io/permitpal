package model

import (
	"fmt"
	"math"
	"time"
)

// AppointmentWarningKind names why a road-test appointment looks unrealistic.
type AppointmentWarningKind string

const (
	// WarnNoPermitDate means the appointment cannot be checked against the
	// permit hold or the driver's pace until the permit issue date is set.
	WarnNoPermitDate AppointmentWarningKind = "no_permit_date"
	// WarnBeforeEligible means the appointment falls before the permit hold ends.
	WarnBeforeEligible AppointmentWarningKind = "before_eligible"
	// WarnTotalHours means the 60 credited total hours will not be reached by
	// the appointment at the observed pace.
	WarnTotalHours AppointmentWarningKind = "total_hours"
	// WarnNightHours is WarnTotalHours for the 10 credited night hours.
	WarnNightHours AppointmentWarningKind = "night_hours"
)

// AppointmentWarning is one reason to doubt the appointment date.
type AppointmentWarning struct {
	Kind    AppointmentWarningKind
	Message string
}

// RoadTestAppointment counts down to the saved road-test appointment and
// lists what makes that date unrealistic. Date is nil when none is saved.
// DaysLeft is 0 on the day itself and after it; Passed marks a date that is
// already behind, which gets no warnings because there is nothing to change.
type RoadTestAppointment struct {
	Date     *time.Time
	DaysLeft int
	Passed   bool
	Warnings []AppointmentWarning
}

// NewRoadTestAppointment checks the appointment against the permit hold and
// against the credited hours at the observed pace, the same pace and weekly
// cap that EstimateReadyDate uses. Without a permit issue date neither check
// is possible, so a single warning asks for it. With no usable pace yet,
// meaning the permit was issued today, the hours check is skipped.
func NewRoadTestAppointment(profile Profile, credit HourCredit, now time.Time) RoadTestAppointment {
	if profile.RoadTestDate == nil {
		return RoadTestAppointment{}
	}
	today, day := startOfDay(now), startOfDay(*profile.RoadTestDate)
	days := int(math.Round(day.Sub(today).Hours() / 24))
	appointment := RoadTestAppointment{Date: &day, DaysLeft: max(days, 0), Passed: days < 0}
	if appointment.Passed {
		return appointment
	}
	if profile.PermitIssueDate == nil {
		appointment.Warnings = append(appointment.Warnings, AppointmentWarning{
			Kind:    WarnNoPermitDate,
			Message: fmt.Sprintf("Add your permit issue date to check this date against the %d-month hold and your pace.", MinPermitHoldMonths),
		})
		return appointment
	}
	start := startOfDay(*profile.PermitIssueDate)
	if holdEnd := permitHoldEnd(start); holdEnd.After(day) {
		appointment.Warnings = append(appointment.Warnings, AppointmentWarning{
			Kind:    WarnBeforeEligible,
			Message: "This is before the permit hold ends. The test date must be " + DisplayDate(&holdEnd) + " or later.",
		})
	}
	elapsed := today.Sub(start).Hours() / 24
	if elapsed <= 0 {
		return appointment
	}
	totalPace, nightPace := profile.TotalHours/elapsed, profile.NightHours/elapsed
	// A requirement with no recorded progress is never met, and
	// projectedCreditDays would not return for it, so it is warned about
	// without projecting.
	warn := func(kind AppointmentWarningKind, label string, required, pace, totalTarget, nightTarget float64) {
		if pace <= 0 {
			appointment.Warnings = append(appointment.Warnings, AppointmentWarning{
				Kind:    kind,
				Message: fmt.Sprintf("No %s hours are recorded yet, so %g %s hours cannot be reached by this date.", label, required, label),
			})
			return
		}
		if projected := projectedCreditDays(credit, today, totalPace, nightPace, totalTarget, nightTarget); projected > days {
			ready := today.AddDate(0, 0, projected)
			appointment.Warnings = append(appointment.Warnings, AppointmentWarning{
				Kind:    kind,
				Message: fmt.Sprintf("At your current pace you reach %g %s hours on %s, after this date.", required, label, DisplayDate(&ready)),
			})
		}
	}
	if credit.TotalHours < TotalHoursRequired {
		warn(WarnTotalHours, "total", TotalHoursRequired, totalPace, TotalHoursRequired, 0)
	}
	if credit.NightHours < NightHoursRequired {
		warn(WarnNightHours, "night", NightHoursRequired, nightPace, 0, NightHoursRequired)
	}
	return appointment
}
