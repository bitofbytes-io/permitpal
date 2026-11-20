package model

import (
	"math"
	"time"
)

// HourEntry is a cumulative snapshot, not a drive. A zero RecordedOn marks
// an undated opening balance. Dated entries retain the server's local save
// date even if APP_TIMEZONE later changes.
type HourEntry struct {
	RecordedAt  time.Time
	RecordedOn  time.Time
	TotalTenths int
	NightTenths int
}

func hourTenths(hours float64) int { return int(math.Round(hours * 10)) }

// AppendHourEntry compares against the stored profile while its repository
// lock is held. Identical submissions and permit-only changes add no hours.
func AppendHourEntry(previous, next Profile, at time.Time) []HourEntry {
	entries := ReconciledHourEntries(previous)

	if hourTenths(previous.TotalHours) != hourTenths(next.TotalHours) || hourTenths(previous.NightHours) != hourTenths(next.NightHours) {
		entries = append(entries, HourEntry{RecordedAt: at, RecordedOn: startOfDay(at), TotalTenths: hourTenths(next.TotalHours), NightTenths: hourTenths(next.NightHours)})
	}
	return entries
}

// ReconciledHourEntries treats writes from an older binary during rollout as
// undated changes. It never invents a submission date for those writes.
func ReconciledHourEntries(profile Profile) []HourEntry {
	entries := append([]HourEntry(nil), profile.HourEntries...)
	total, night := hourTenths(profile.TotalHours), hourTenths(profile.NightHours)
	if len(entries) == 0 {
		if total != 0 || night != 0 {
			entries = append(entries, HourEntry{TotalTenths: total, NightTenths: night})
		}
	} else if last := entries[len(entries)-1]; last.TotalTenths != total || last.NightTenths != night {
		entries = append(entries, HourEntry{TotalTenths: total, NightTenths: night})
	}
	return entries
}
