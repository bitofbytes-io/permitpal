package model

import "testing"

func TestRoadTestReadinessNotReadyUntilHoursAndSkillsAreMet(t *testing.T) {
	now := dateOn("2026-10-27")
	fresh := NewDashboard(NewDriver("aiden"), NewDriverProfile(now), DefaultRequirements(now), now)
	if fresh.Readiness.Ready || fresh.Readiness.TotalHoursLeft != TotalHoursRequired || fresh.Readiness.NightHoursLeft != NightHoursRequired {
		t.Fatalf("fresh readiness = %+v", fresh.Readiness)
	}
	if got, want := len(fresh.Readiness.Groups), 3; got != want {
		t.Fatalf("groups = %d, want %d", got, want)
	}
	var listed int
	for _, group := range fresh.Readiness.Groups {
		listed += len(group.Items)
	}
	if listed != len(fresh.Requirements) {
		t.Fatalf("listed %d skills, want all %d", listed, len(fresh.Requirements))
	}
	if fresh.Readiness.Groups[0].Category != categoryManeuvers || fresh.Readiness.Groups[0].Items[0].Key != "quick-stop" {
		t.Fatalf("first group = %+v", fresh.Readiness.Groups[0])
	}

	reqs := DefaultRequirements(now)
	for i := range reqs {
		reqs[i].Rating = RatingGood
	}
	byKey := map[string]int{}
	for i, req := range reqs {
		byKey[req.Key] = i
	}
	reqs[byKey["turn-about"]].Rating = RatingNotRated
	reqs[byKey["backing"]].Rating = RatingFair
	reqs[byKey["left-turns"]].Rating = RatingBad
	reqs[byKey["following"]].Rating = RatingNotRated
	reqs[byKey["quick-stop"]].Rating = RatingGood
	partialProfile := Profile{TotalHours: 42, NightHours: 6.5}
	partial := NewRoadTestReadiness(CreditHours(partialProfile), reqs)
	if partial.Ready || partial.TotalHoursLeft != 18 || partial.NightHoursLeft != 3.5 {
		t.Fatalf("partial = %+v", partial)
	}
	if len(partial.Groups) != 3 || partial.Groups[0].Category != categoryManeuvers || partial.Groups[1].Category != categoryIntersections || partial.Groups[2].Category != categoryGeneralDriving {
		t.Fatalf("groups = %+v", partial.Groups)
	}
	if keys := requirementKeys(partial.Groups[0].Items); keys[0] != "turn-about" || keys[1] != "backing" {
		t.Fatalf("maneuvers = %v", keys)
	}
	if partial.Groups[1].Items[0].Key != "left-turns" || partial.Groups[2].Items[0].Key != "following" {
		t.Fatalf("later groups = %+v", partial.Groups)
	}
	for _, group := range partial.Groups {
		for _, item := range group.Items {
			if item.Key == "quick-stop" || item.Rating == RatingGood {
				t.Fatalf("mastered skill listed: %+v", item)
			}
		}
	}

	readyProfile := Profile{TotalHours: 70, NightHours: 12}
	ready := NewRoadTestReadiness(CreditHours(readyProfile), mastered(DefaultRequirements(now)))
	if !ready.Ready || ready.TotalHoursLeft != 0 || ready.NightHoursLeft != 0 || len(ready.Groups) != 0 {
		t.Fatalf("over-credit ready = %+v", ready)
	}
}

func TestRoadTestReadinessUsesCreditedHours(t *testing.T) {
	// Sixty recorded hours in one week credit only the weekly cap.
	capped := saveHours(Profile{}, 60, 10, dateOn("2026-03-02"))
	credit := CreditHours(capped)
	if credit.TotalHours != 10 || credit.NightHours != 10 {
		t.Fatalf("credit = %+v, want 10/10", credit)
	}
	got := NewRoadTestReadiness(credit, mastered(DefaultRequirements(dateOn("2026-03-02"))))
	if got.Ready || got.TotalHoursLeft != 50 || got.NightHoursLeft != 0 || len(got.Groups) != 0 {
		t.Fatalf("capped readiness = %+v", got)
	}

	// Night can be shorter than the total that is still required.
	shortNight := NewRoadTestReadiness(HourCredit{TotalHours: 55, NightHours: 2}, mastered(DefaultRequirements(dateOn("2026-03-02"))))
	if shortNight.Ready || shortNight.TotalHoursLeft != 5 || shortNight.NightHoursLeft != 8 {
		t.Fatalf("short night = %+v", shortNight)
	}

	spread := Profile{}
	start := dateOn("2026-03-02")
	for i, total := range []float64{10, 20, 30, 40, 50, 60} {
		night := []float64{2, 4, 6, 8, 9, 10}[i]
		spread = saveHours(spread, total, night, start.AddDate(0, 0, 7*i))
	}
	spreadCredit := CreditHours(spread)
	if spreadCredit.TotalHours != 60 || spreadCredit.NightHours != 10 {
		t.Fatalf("spread credit = %+v", spreadCredit)
	}
	if got := NewRoadTestReadiness(spreadCredit, mastered(DefaultRequirements(start))); !got.Ready {
		t.Fatalf("spread readiness = %+v", got)
	}
}

func TestRoadTestReadinessGroupsUnmasteredSkills(t *testing.T) {
	reqs := []Requirement{
		{Key: "mystery", Title: "Mystery skill", Rating: RatingFair, SortOrder: 1},
		{Key: "quick-stop", Title: "Quick stop", Rating: RatingBad, SortOrder: 3},
		{Key: "three-point-turn", Title: "Three point turn", Rating: RatingNotRated, SortOrder: 2},
		{Key: "parking", Title: "Parking", Rating: RatingGood, SortOrder: 4},
	}
	got := NewRoadTestReadiness(HourCredit{TotalHours: 60, NightHours: 10}, reqs)
	if got.Ready || got.TotalHoursLeft != 0 || got.NightHoursLeft != 0 {
		t.Fatalf("hours should be met: %+v", got)
	}
	if len(got.Groups) != 2 || got.Groups[0].Category != categoryManeuvers || got.Groups[1].Category != categoryOther {
		t.Fatalf("groups = %+v", got.Groups)
	}
	if keys := requirementKeys(got.Groups[0].Items); len(keys) != 2 || keys[0] != "three-point-turn" || keys[1] != "quick-stop" {
		t.Fatalf("maneuvers = %v", keys)
	}
	if got.Groups[1].Items[0].Key != "mystery" {
		t.Fatalf("other = %+v", got.Groups[1])
	}
}

func TestRoadTestReadinessCountsARemainingTenth(t *testing.T) {
	got := NewRoadTestReadiness(HourCredit{TotalHours: 59.9, NightHours: 10}, mastered(DefaultRequirements(dateOn("2026-10-27"))))
	if got.Ready || FormatHours(got.TotalHoursLeft) != "0.1" || got.NightHoursLeft != 0 || len(got.Groups) != 0 {
		t.Fatalf("%+v", got)
	}
}

func mastered(requirements []Requirement) []Requirement {
	out := append([]Requirement(nil), requirements...)
	for i := range out {
		out[i].Rating = RatingGood
	}
	return out
}

func requirementKeys(requirements []Requirement) []string {
	keys := make([]string, len(requirements))
	for i, req := range requirements {
		keys[i] = req.Key
	}
	return keys
}
