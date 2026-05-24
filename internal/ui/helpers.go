package ui

import (
	"fmt"

	"github.com/drywaters/permitpal/internal/model"
)

func progressStyle(percent int) string {
	return fmt.Sprintf("--progress:%d%%", percent)
}

func requirementRowClass(status model.RequirementStatus) string {
	if status == model.StatusMastered {
		return "skill-row skill-row--mastered"
	}
	return "skill-row"
}

func odometerChars(value float64) []string {
	formatted := fmt.Sprintf("%04.1f", value)
	chars := make([]string, 0, len(formatted))
	for _, char := range formatted {
		chars = append(chars, string(char))
	}
	return chars
}
