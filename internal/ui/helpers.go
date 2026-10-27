package ui

import (
	"fmt"

	"github.com/drywaters/permitpal/internal/model"
)

// recordedHoursPattern matches the 1,000-hour cumulative input ceiling.
const recordedHoursPattern = `(1000(\.0)?|[0-9]{1,3}(\.[0-9])?|\.[0-9])`

func progressStyle(percent int) string {
	return fmt.Sprintf("--progress:%d%%", percent)
}

// masteredOn is the sign-off date of a good rating, or empty when the skill
// is not mastered.
func masteredOn(req model.Requirement) string {
	if req.Rating != model.RatingGood || req.RatedOn == nil {
		return ""
	}
	return model.DateValue(req.RatedOn)
}

// masteredStatus is the checklist status for a mastered skill, for example
// "Mastered Oct 27, 2026". It is empty until the skill has a sign-off date.
func masteredStatus(req model.Requirement) string {
	if masteredOn(req) == "" {
		return ""
	}
	return "Mastered " + model.DisplayDate(req.RatedOn)
}

func requirementRowClass(rating model.RequirementRating) string {
	switch rating {
	case model.RatingBad, model.RatingFair, model.RatingGood:
		return "skill-row skill-row--" + string(rating)
	default:
		return "skill-row"
	}
}

// estimateLabel introduces the ready date: an hours-pace projection, or the
// fixed day the permit hold ends. The hold label claims no more than that,
// since the age requirement is not tracked.
func estimateLabel(waitingOn model.ReadyConstraint) string {
	if waitingOn == model.ConstraintPermitHold {
		return "Hold ends"
	}
	return "On pace for"
}

// estimateHint is the ready estimate's text when it has no date. Once the
// hour and skill requirements are met, the sign agrees with the readiness card.
func estimateHint(dashboard model.Dashboard) string {
	if dashboard.Readiness.Ready && dashboard.ReadyEstimate.Date == nil {
		return "Hours and skills are met"
	}
	return dashboard.ReadyEstimate.Hint
}

// estimateNote names the constraint that sets the ready date.
func estimateNote(waitingOn model.ReadyConstraint) string {
	switch waitingOn {
	case model.ConstraintPermitHold:
		return fmt.Sprintf("Waiting on %d-month hold", model.MinPermitHoldMonths)
	case model.ConstraintWeeklyCap:
		return fmt.Sprintf("Waiting on %d-hour weekly cap", model.WeeklyHourCap)
	case model.ConstraintNightHours:
		return "Waiting on night hours"
	case model.ConstraintTotalHours:
		return "Waiting on total hours"
	default:
		return ""
	}
}

func daysWord(days int) string {
	if days == 1 {
		return "day"
	}
	return "days"
}

func driveNudgeClass(stale bool) string {
	if stale {
		return "drive-nudge drive-nudge--stale"
	}
	return "drive-nudge"
}

func odometerChars(value float64) []string {
	formatted := fmt.Sprintf("%04.1f", value)
	chars := make([]string, 0, len(formatted))
	for _, char := range formatted {
		chars = append(chars, string(char))
	}
	return chars
}
