package model

import "time"

// ApplyRequirementUpdate records one checklist save.
//
// Good is mastered. An omitted date becomes today, and a submitted date is
// kept so the sign-off date stays editable.
//
// Bad and fair still need practice. Leaving good for either of them clears
// the sign-off date when the submitted date is empty or still the mastered
// date. A different date submitted in that same save is the practice
// rating's last-rated date.
//
// Not rated always clears the date, including a date submitted with the clear.
func ApplyRequirementUpdate(existing Requirement, rating RequirementRating, ratedOn *time.Time, notes string, today time.Time) Requirement {
	previouslyMastered := existing.Rating == RatingGood
	previousDate := DateValue(existing.RatedOn)
	existing.Rating = rating
	existing.Notes = notes
	switch rating {
	case RatingNotRated:
		existing.RatedOn = nil
	case RatingGood:
		existing.RatedOn = dateOrToday(ratedOn, today)
	case RatingBad, RatingFair:
		if previouslyMastered && (ratedOn == nil || DateValue(ratedOn) == previousDate) {
			existing.RatedOn = nil
			break
		}
		existing.RatedOn = dateOrToday(ratedOn, today)
	default:
		existing.RatedOn = ratedOn
	}
	return existing
}

func dateOrToday(ratedOn *time.Time, today time.Time) *time.Time {
	if ratedOn != nil {
		return ratedOn
	}
	stamp := today
	return &stamp
}
