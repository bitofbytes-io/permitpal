package ui

import (
	"fmt"

	"github.com/drywaters/permitpal/internal/model"
)

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
// fixed day the permit hold ends.
func estimateLabel(waitingOn model.ReadyConstraint) string {
	if waitingOn == model.ConstraintPermitHold {
		return "Eligible on"
	}
	return "On pace for"
}

// estimateNote names the constraint that sets the ready date.
func estimateNote(waitingOn model.ReadyConstraint) string {
	switch waitingOn {
	case model.ConstraintPermitHold:
		return fmt.Sprintf("Waiting on %d-month hold", model.MinPermitHoldMonths)
	case model.ConstraintNightHours:
		return "Waiting on night hours"
	case model.ConstraintTotalHours:
		return "Waiting on total hours"
	default:
		return ""
	}
}

func odometerChars(value float64) []string {
	formatted := fmt.Sprintf("%04.1f", value)
	chars := make([]string, 0, len(formatted))
	for _, char := range formatted {
		chars = append(chars, string(char))
	}
	return chars
}
