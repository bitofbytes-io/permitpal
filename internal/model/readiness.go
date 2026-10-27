package model

import "sort"

const (
	categoryManeuvers      = "Maneuvers"
	categoryIntersections  = "Intersections"
	categoryGeneralDriving = "General driving"
	categoryOther          = "Other"
)

// readinessCategoryOrder is the road-test order groups appear in. Other holds
// any skill key this list does not name, so a saved checklist item is never
// dropped from the summary.
var readinessCategoryOrder = []string{categoryManeuvers, categoryIntersections, categoryGeneralDriving, categoryOther}

// readinessCategoryByKey groups the current score-sheet skills and the
// pre-migration checklist keys. A good rating is mastered and is left out of
// the summary.
var readinessCategoryByKey = map[string]string{
	"quick-stop":           categoryManeuvers,
	"turn-about":           categoryManeuvers,
	"three-point-turn":     categoryManeuvers,
	"stop-on-grade":        categoryManeuvers,
	"start-on-grade":       categoryManeuvers,
	"backing":              categoryManeuvers,
	"approach-corner":      categoryIntersections,
	"right-turns":          categoryIntersections,
	"left-turns":           categoryIntersections,
	"left-right-turns":     categoryIntersections,
	"turn-lanes":           categoryIntersections,
	"traffic-lights":       categoryIntersections,
	"stop-signs":           categoryIntersections,
	"yield-caution-lights": categoryIntersections,
	"use-of-controls":      categoryGeneralDriving,
	"starts":               categoryGeneralDriving,
	"starting-the-car":     categoryGeneralDriving,
	"posture":              categoryGeneralDriving,
	"forward-movement":     categoryGeneralDriving,
	"use-of-lane":          categoryGeneralDriving,
	"lane-changes":         categoryGeneralDriving,
	"use-of-brake":         categoryGeneralDriving,
	"following":            categoryGeneralDriving,
	"attention":            categoryGeneralDriving,
	"driver-courtesy":      categoryGeneralDriving,
	"parking":              categoryGeneralDriving,
}

// ReadinessGroup is one category of skills that are not yet rated Good.
type ReadinessGroup struct {
	Category string
	Items    []Requirement
}

// RoadTestReadiness is what is left before the hour and skill requirements
// are met. TotalHoursLeft and NightHoursLeft come from credited hours, so
// the weekly cap is the one CreditHours already applied.
type RoadTestReadiness struct {
	Ready          bool
	TotalHoursLeft float64
	NightHoursLeft float64
	Groups         []ReadinessGroup
}

// NewRoadTestReadiness lists credited hours still short of 60 total and 10
// night, and every skill that is not rated Good. credit must be the
// CreditHours result; this does not replay the weekly cap.
func NewRoadTestReadiness(credit HourCredit, requirements []Requirement) RoadTestReadiness {
	readiness := RoadTestReadiness{
		TotalHoursLeft: hoursShort(credit.TotalHours, TotalHoursRequired),
		NightHoursLeft: hoursShort(credit.NightHours, NightHoursRequired),
	}
	grouped := map[string][]Requirement{}
	for _, req := range requirements {
		if req.Rating == RatingGood {
			continue
		}
		category := readinessCategoryByKey[req.Key]
		if category == "" {
			category = categoryOther
		}
		grouped[category] = append(grouped[category], req)
	}
	for _, category := range readinessCategoryOrder {
		items := grouped[category]
		if len(items) == 0 {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].SortOrder < items[j].SortOrder
		})
		readiness.Groups = append(readiness.Groups, ReadinessGroup{Category: category, Items: items})
	}
	readiness.Ready = readiness.TotalHoursLeft == 0 && readiness.NightHoursLeft == 0 && len(readiness.Groups) == 0
	return readiness
}

// hoursShort is required minus credited, in the journal's tenth-hour units.
// Zero means the credited total already meets the requirement.
func hoursShort(credited, required float64) float64 {
	short := hourTenths(required) - hourTenths(credited)
	if short <= 0 {
		return 0
	}
	return float64(short) / 10
}
