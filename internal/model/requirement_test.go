package model

import (
	"testing"
	"time"
)

func TestApplyRequirementUpdateMasteredDate(t *testing.T) {
	today := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	edited := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	other := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	base := Requirement{Key: "quick-stop", Rating: RatingNotRated, SortOrder: 1}

	mastered := ApplyRequirementUpdate(base, RatingGood, nil, "signed off", today)
	if mastered.Rating != RatingGood || DateValue(mastered.RatedOn) != "2026-10-27" || mastered.Notes != "signed off" {
		t.Fatalf("mastered without a date: %+v", mastered)
	}
	kept := ApplyRequirementUpdate(base, RatingGood, &edited, "", today)
	if DateValue(kept.RatedOn) != "2026-08-15" {
		t.Fatalf("edited mastered date: %s", DateValue(kept.RatedOn))
	}
	if stayed := ApplyRequirementUpdate(kept, RatingGood, &edited, "note", today); DateValue(stayed.RatedOn) != "2026-08-15" || stayed.Notes != "note" {
		t.Fatalf("staying mastered: %+v", stayed)
	}

	// Changing back to a practice rating clears the sign-off date.
	for _, rating := range []RequirementRating{RatingFair, RatingBad} {
		cleared := ApplyRequirementUpdate(kept, rating, &edited, "again", today)
		if cleared.Rating != rating || cleared.RatedOn != nil || cleared.Notes != "again" {
			t.Fatalf("%s with the mastered date: %+v", rating, cleared)
		}
		if blank := ApplyRequirementUpdate(kept, rating, nil, "", today); blank.RatedOn != nil {
			t.Fatalf("%s with an empty date stamped today: %s", rating, DateValue(blank.RatedOn))
		}
	}
	practiced := ApplyRequirementUpdate(kept, RatingFair, &other, "", today)
	if practiced.Rating != RatingFair || DateValue(practiced.RatedOn) != "2026-09-01" {
		t.Fatalf("new practice date: %+v", practiced)
	}

	notRated := ApplyRequirementUpdate(kept, RatingNotRated, &edited, "keep note", today)
	if notRated.Rating != RatingNotRated || notRated.RatedOn != nil || notRated.Notes != "keep note" {
		t.Fatalf("not rated: %+v", notRated)
	}

	fair := ApplyRequirementUpdate(base, RatingFair, nil, "", today)
	if fair.Rating != RatingFair || DateValue(fair.RatedOn) != "2026-10-27" {
		t.Fatalf("first practice rating: %+v", fair)
	}
}
