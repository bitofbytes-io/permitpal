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

func odometerChars(value float64) []string {
	formatted := fmt.Sprintf("%04.1f", value)
	chars := make([]string, 0, len(formatted))
	for _, char := range formatted {
		chars = append(chars, string(char))
	}
	return chars
}
