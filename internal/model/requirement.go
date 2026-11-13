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

type RequirementRating string

const (
	RatingNotRated RequirementRating = "not_rated"
	RatingGood     RequirementRating = "good"
	RatingBad      RequirementRating = "bad"
	RatingFair     RequirementRating = "fair"
)

type Requirement struct {
	Key         string
	Title       string
	Description string
	Rating      RequirementRating
	RatedOn     *time.Time
	Notes       string
	SortOrder   int
	UpdatedAt   time.Time
}

func ParseRating(value string) (RequirementRating, bool) {
	switch RequirementRating(value) {
	case "", RatingNotRated:
		return RatingNotRated, true
	case RatingBad, RatingFair, RatingGood:
		return RequirementRating(value), true
	default:
		return "", false
	}
}

func DefaultRequirements(now time.Time) []Requirement {
	items := []struct{ key, title, description string }{
		{"quick-stop", "Quick stop", "Stop quickly and safely on the examiner's signal, keeping the car straight and under control."},
		{"turn-about", "Turn about", "Check traffic, signal, and complete the turn safely with control and room to maneuver."},
		{"stop-on-grade", "Stop on grade", "Stop under control, turn the wheels correctly for the slope, and set the parking brake."},
		{"start-on-grade", "Start on grade", "Check traffic, release the parking brake, and move smoothly without rolling backward."},
		{"backing", "Backing", "Look behind you and back slowly in a straight line while checking for traffic."},
		{"approach-corner", "Approach corner", "Check traffic, signal early, and slow to a safe speed before reaching the corner."},
		{"right-turns", "Right turns", "Signal, yield to pedestrians and traffic, and turn into the proper lane without swinging wide."},
		{"left-turns", "Left turns", "Signal, yield to oncoming traffic and pedestrians, and finish in the proper lane."},
		{"traffic-lights", "Traffic lights", "Observe signals, stop behind the line, and check traffic before proceeding on green."},
		{"use-of-controls", "Use of controls", "Use mirrors, signals, steering, and other vehicle controls correctly without distraction."},
		{"starts", "Starts", "Check mirrors and blind spots, signal when needed, and accelerate smoothly into traffic."},
		{"use-of-lane", "Use of lane", "Keep a steady lane position and check mirrors and blind spots before changing lanes."},
		{"use-of-brake", "Use of brake", "Brake smoothly and early enough to stop safely while keeping control."},
		{"following", "Following", "Maintain at least a 2–3 second gap and allow more space in poor conditions."},
		{"attention", "Attention", "Watch the road, scan for hazards, and respond safely to traffic and pedestrians."},
		{"stop-signs", "Stop signs", "Make a complete stop behind the line and check traffic before proceeding."},
		{"parking", "Parking", "Check surroundings, signal, and park safely within the space with the vehicle secured."},
	}
	requirements := make([]Requirement, len(items))
	for i, item := range items {
		requirements[i] = Requirement{Key: item.key, Title: item.title, Description: item.description, Rating: RatingNotRated, SortOrder: i + 1, UpdatedAt: now}
	}
	return requirements
}

func RequirementByKey(requirements []Requirement, key string) (Requirement, bool) {
	for _, req := range requirements {
		if req.Key == key {
			return req, true
		}
	}
	return Requirement{}, false
}

func dateOrToday(ratedOn *time.Time, today time.Time) *time.Time {
	if ratedOn != nil {
		return ratedOn
	}
	stamp := today
	return &stamp
}
