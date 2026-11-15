package model

import "time"

// Milestone is a badge on the way to the hour requirements. Reached is true
// while the recorded hours are at or above the line. On is the entry date of
// the save that crossed it, and is nil when the hours were already there as
// an undated opening balance.
type Milestone struct {
	Key     string
	Label   string
	Reached bool
	On      *time.Time
}

type milestoneLine struct {
	key, label string
	tenths     int
	night      bool
}

// milestoneLines are the badge lines in display order: total hours at 10, 30,
// 45 and 60, then the night requirement.
var milestoneLines = []milestoneLine{
	{key: "total-10", label: "10 total hours", tenths: 100},
	{key: "total-30", label: "30 total hours", tenths: 300},
	{key: "total-45", label: "45 total hours", tenths: 450},
	{key: "total-60", label: "60 total hours", tenths: hourTenths(TotalHoursRequired)},
	{key: "night-10", label: "Night hours complete", tenths: hourTenths(NightHoursRequired), night: true},
}

// Milestones replays the recorded cumulative snapshots. A badge is dated by
// the entry that started the run of snapshots now at or above its line, so a
// correction that drops back below the line removes the badge, and the save
// that later crosses it again re-dates it. Like the hour gauges, it counts
// recorded totals, before the weekly credit cap.
func Milestones(profile Profile) []Milestone {
	entries := ReconciledHourEntries(profile)
	milestones := make([]Milestone, len(milestoneLines))
	for i, line := range milestoneLines {
		milestones[i] = Milestone{Key: line.key, Label: line.label}
		for _, entry := range entries {
			tenths := entry.TotalTenths
			if line.night {
				tenths = entry.NightTenths
			}
			if tenths < line.tenths {
				milestones[i].Reached, milestones[i].On = false, nil
				continue
			}
			if milestones[i].Reached {
				continue
			}
			milestones[i].Reached = true
			if !entry.RecordedOn.IsZero() {
				on := entry.RecordedOn
				milestones[i].On = &on
			}
		}
	}
	return milestones
}

// NewMilestones lists the badges a save earned: reached by next but not by
// previous. A badge lost to a correction and earned again counts as new.
func NewMilestones(previous, next Profile) []Milestone {
	before := Milestones(previous)
	var earned []Milestone
	for i, milestone := range Milestones(next) {
		if milestone.Reached && !before[i].Reached {
			earned = append(earned, milestone)
		}
	}
	return earned
}
