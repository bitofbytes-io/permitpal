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

func odometerChars(value float64) []string {
	formatted := fmt.Sprintf("%04.1f", value)
	chars := make([]string, 0, len(formatted))
	for _, char := range formatted {
		chars = append(chars, string(char))
	}
	return chars
}
