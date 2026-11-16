package model

import (
	"slices"
	"testing"
	"time"
)

func reachedMilestones(p Profile) map[string]string {
	got := map[string]string{}
	for _, m := range Milestones(p) {
		if m.Reached {
			got[m.Key] = DateValue(m.On)
		}
	}
	return got
}

func wantMilestones(t *testing.T, p Profile, want map[string]string) {
	t.Helper()
	got := reachedMilestones(p)
	if len(got) != len(want) {
		t.Fatalf("reached=%v, want %v", got, want)
	}
	for key, date := range want {
		if have, ok := got[key]; !ok || have != date {
			t.Fatalf("reached=%v, want %v", got, want)
		}
	}
}

func milestoneKeys(ms []Milestone) []string {
	var keys []string
	for _, m := range ms {
		keys = append(keys, m.Key)
	}
	return keys
}

func TestMilestonesDateTheEntryThatCrossedEachLine(t *testing.T) {
	p := Profile{}
	if len(Milestones(p)) != 5 || len(reachedMilestones(p)) != 0 {
		t.Fatalf("empty profile reached %v", reachedMilestones(p))
	}
	p = saveHours(p, 9.9, 0, dateOn("2026-01-05"))
	wantMilestones(t, p, map[string]string{})
	p = saveHours(p, 10, 2, dateOn("2026-01-06"))
	wantMilestones(t, p, map[string]string{"total-10": "2026-01-06"})
	// One save can cross several lines; each takes that save's date.
	p = saveHours(p, 46, 10, dateOn("2026-02-01"))
	wantMilestones(t, p, map[string]string{
		"total-10": "2026-01-06", "total-30": "2026-02-01", "total-45": "2026-02-01", "night-10": "2026-02-01",
	})
	// Staying above a line later does not move its date.
	p = saveHours(p, 50, 12, dateOn("2026-02-08"))
	p = saveHours(p, 60, 12, dateOn("2026-03-01"))
	wantMilestones(t, p, map[string]string{
		"total-10": "2026-01-06", "total-30": "2026-02-01", "total-45": "2026-02-01", "total-60": "2026-03-01", "night-10": "2026-02-01",
	})
}

func TestMilestonesDropAfterCorrectionAndRedateWhenCrossedAgain(t *testing.T) {
	p := saveHours(Profile{}, 31, 4, dateOn("2026-01-10"))
	p = saveHours(p, 45, 10, dateOn("2026-01-20"))
	wantMilestones(t, p, map[string]string{"total-10": "2026-01-10", "total-30": "2026-01-10", "total-45": "2026-01-20", "night-10": "2026-01-20"})

	// Correcting down removes only the badges now above the total.
	p = saveHours(p, 44.9, 9.9, dateOn("2026-01-21"))
	wantMilestones(t, p, map[string]string{"total-10": "2026-01-10", "total-30": "2026-01-10"})
	p = saveHours(p, 29, 9.9, dateOn("2026-01-22"))
	wantMilestones(t, p, map[string]string{"total-10": "2026-01-10"})

	// Crossing again is dated by the new save, not the original one.
	p = saveHours(p, 30, 10, dateOn("2026-02-03"))
	wantMilestones(t, p, map[string]string{"total-10": "2026-01-10", "total-30": "2026-02-03", "night-10": "2026-02-03"})
}

func TestMilestonesUndatedOpeningBalance(t *testing.T) {
	p := Profile{TotalHours: 34.5, NightHours: 6}
	got := Milestones(p)
	if !got[0].Reached || got[0].On != nil || !got[1].Reached || got[1].On != nil || got[2].Reached {
		t.Fatalf("opening balance milestones=%+v", got)
	}
	// A later save dates only the lines it crosses.
	p = saveHours(p, 46, 10, dateOn("2026-10-21"))
	wantMilestones(t, p, map[string]string{"total-10": "", "total-30": "", "total-45": "2026-10-21", "night-10": "2026-10-21"})
	// A write from an older binary is reconciled as undated, never dated.
	p.TotalHours = 61
	wantMilestones(t, p, map[string]string{"total-10": "", "total-30": "", "total-45": "2026-10-21", "total-60": "", "night-10": "2026-10-21"})
}

func TestNewMilestonesReportsOnlyBadgesTheSaveEarned(t *testing.T) {
	at := func(day string) time.Time { return dateOn(day).Add(9 * time.Hour) }
	p := saveHours(Profile{}, 5, 0, at("2026-01-01"))
	next := saveHours(p, 31, 10, at("2026-01-02"))
	if got := milestoneKeys(NewMilestones(next, at("2026-01-02"))); !slices.Equal(got, []string{"total-10", "total-30", "night-10"}) {
		t.Fatalf("earned=%v", got)
	}
	// Resubmitting, or adding hours short of the next line, celebrates nothing.
	same := saveHours(next, 31, 10, at("2026-01-03"))
	if got := NewMilestones(same, at("2026-01-03")); len(got) != 0 {
		t.Fatalf("unchanged save earned %v", milestoneKeys(got))
	}
	short := saveHours(next, 44.9, 10, at("2026-01-03"))
	if got := NewMilestones(short, at("2026-01-03")); len(got) != 0 {
		t.Fatalf("short save earned %v", milestoneKeys(got))
	}
	// A correction below a line is not a milestone; crossing it again is.
	down := saveHours(next, 29, 10, at("2026-01-03"))
	if got := NewMilestones(down, at("2026-01-03")); len(got) != 0 {
		t.Fatalf("correction earned %v", milestoneKeys(got))
	}
	again := saveHours(down, 30, 10, at("2026-01-04"))
	if got := milestoneKeys(NewMilestones(again, at("2026-01-04"))); !slices.Equal(got, []string{"total-30"}) {
		t.Fatalf("re-crossing earned %v", got)
	}
	// Hours already present as an undated balance are not newly earned.
	opening := Profile{TotalHours: 34.5, NightHours: 6}
	if got := NewMilestones(saveHours(opening, 36, 6, at("2026-01-05")), at("2026-01-05")); len(got) != 0 {
		t.Fatalf("opening balance earned %v", milestoneKeys(got))
	}
}

// Two saves can both read the same stored hours. The one that wrote the
// crossing entry is celebrated; the other, which adds nothing to the journal
// and gets back the reconciled profile, is not.
func TestNewMilestonesCelebratesOnlyTheSaveThatWroteTheCrossing(t *testing.T) {
	first := dateOn("2026-02-01").Add(9 * time.Hour)
	second := first.Add(time.Second)
	before := saveHours(Profile{}, 5, 0, dateOn("2026-01-01"))
	afterFirst := saveHours(before, 10, 0, first)
	afterSecond := saveHours(afterFirst, 10, 0, second) // Reconciled under the lock: no new entry.
	if got := milestoneKeys(NewMilestones(afterFirst, first)); !slices.Equal(got, []string{"total-10"}) {
		t.Fatalf("first save earned %v", got)
	}
	if got := NewMilestones(afterSecond, second); len(got) != 0 {
		t.Fatalf("overlapping save earned %v", milestoneKeys(got))
	}
}
