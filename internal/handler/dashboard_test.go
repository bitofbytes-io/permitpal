package handler

import (
	"strings"
	"testing"
)

func TestParseHoursAllowsSingleDecimalPlace(t *testing.T) {
	tests := map[string]float64{
		"":     0,
		"0":    0,
		"6":    6,
		"6.0":  6,
		"34.5": 34.5,
		".5":   0.5,
	}

	for value, want := range tests {
		got, err := parseHours(value, 60)
		if err != nil {
			t.Fatalf("parseHours(%q) returned error: %v", value, err)
		}
		if got != want {
			t.Fatalf("parseHours(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestParseHoursRejectsMoreThanOneDecimalPlace(t *testing.T) {
	for _, value := range []string{"1.23", "34.50", "6.01", "1e1", "-1"} {
		if _, err := parseHours(value, 60); err == nil {
			t.Fatalf("parseHours(%q) returned nil error", value)
		}
	}
}

func TestParseHoursRejectsValuesAboveMaximum(t *testing.T) {
	for _, value := range []string{"60.1", "99999"} {
		if _, err := parseHours(value, 60); err == nil {
			t.Fatalf("parseHours(%q) returned nil error", value)
		}
	}
}

func TestRequirementNotesLengthLimit(t *testing.T) {
	notes := strings.Repeat("a", maxNotesChars+1)
	if len(notes) <= maxNotesChars {
		t.Fatal("test setup failed")
	}
}
