package model

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func appointmentKinds(a RoadTestAppointment) []AppointmentWarningKind {
	var kinds []AppointmentWarningKind
	for _, warning := range a.Warnings {
		kinds = append(kinds, warning.Kind)
	}
	return kinds
}

func TestRoadTestAppointmentCountdownAndWarnings(t *testing.T) {
	now := time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC)
	date := func(s string) *time.Time { d := dateOn(s); return &d }
	tests := []struct {
		name         string
		profile      Profile
		wantDays     int
		wantPassed   bool
		wantKinds    []AppointmentWarningKind
		wantContains []string
	}{
		{
			name:    "no appointment",
			profile: Profile{PermitIssueDate: date("2026-01-15"), TotalHours: 20, NightHours: 2},
		},
		{
			name:       "passed appointment is not warned about",
			profile:    Profile{PermitIssueDate: date("2026-01-15"), RoadTestDate: date("2026-11-15")},
			wantPassed: true,
		},
		{
			name:         "no permit date",
			profile:      Profile{RoadTestDate: date("2026-12-01"), TotalHours: 20, NightHours: 2},
			wantDays:     15,
			wantKinds:    []AppointmentWarningKind{WarnNoPermitDate},
			wantContains: []string{"Add your permit issue date"},
		},
		{
			name:         "before the hold ends",
			profile:      Profile{PermitIssueDate: date("2026-06-01"), RoadTestDate: date("2027-01-10"), TotalHours: 60, NightHours: 10},
			wantDays:     55,
			wantKinds:    []AppointmentWarningKind{WarnBeforeEligible},
			wantContains: []string{"must be Mar 1, 2027 or later"},
		},
		{
			name:     "on the day the hold ends",
			profile:  Profile{PermitIssueDate: date("2026-02-16"), RoadTestDate: date("2026-11-16"), TotalHours: 60, NightHours: 10},
			wantDays: 0,
		},
		{
			name:     "hours and night hours already met",
			profile:  Profile{PermitIssueDate: date("2026-01-15"), RoadTestDate: date("2026-11-30"), TotalHours: 60, NightHours: 10},
			wantDays: 14,
		},
		{
			name:         "slow pace misses both requirements",
			profile:      Profile{PermitIssueDate: date("2025-01-15"), RoadTestDate: date("2026-11-30"), TotalHours: 20, NightHours: 3},
			wantDays:     14,
			wantKinds:    []AppointmentWarningKind{WarnTotalHours, WarnNightHours},
			wantContains: []string{"reach 60 total hours on", "reach 10 night hours on", "after this date"},
		},
		{
			name:         "only night hours fall short",
			profile:      Profile{PermitIssueDate: date("2025-11-16"), RoadTestDate: date("2027-01-01"), TotalHours: 60, NightHours: 2},
			wantDays:     46,
			wantKinds:    []AppointmentWarningKind{WarnNightHours},
			wantContains: []string{"reach 10 night hours on"},
		},
		{
			name:         "no night hours recorded",
			profile:      Profile{PermitIssueDate: date("2026-01-15"), RoadTestDate: date("2028-01-01"), TotalHours: 60, NightHours: 0},
			wantDays:     411,
			wantKinds:    []AppointmentWarningKind{WarnNightHours},
			wantContains: []string{"No night hours are recorded"},
		},
		{
			name:         "no hours recorded and hold not over",
			profile:      Profile{PermitIssueDate: date("2026-09-01"), RoadTestDate: date("2027-01-01")},
			wantDays:     46,
			wantKinds:    []AppointmentWarningKind{WarnBeforeEligible, WarnTotalHours, WarnNightHours},
			wantContains: []string{"No total hours are recorded", "No night hours are recorded"},
		},
		{
			name:     "far enough out for the current pace",
			profile:  Profile{PermitIssueDate: date("2026-01-15"), RoadTestDate: date("2028-01-01"), TotalHours: 30, NightHours: 5},
			wantDays: 411,
		},
		{
			name:     "permit issued today has no pace yet",
			profile:  Profile{PermitIssueDate: date("2026-11-16"), RoadTestDate: date("2027-08-20"), TotalHours: 5, NightHours: 1},
			wantDays: 277,
		},
		{
			name:      "same day with hours still short",
			profile:   Profile{PermitIssueDate: date("2026-01-15"), RoadTestDate: date("2026-11-16"), TotalHours: 30, NightHours: 5},
			wantDays:  0,
			wantKinds: []AppointmentWarningKind{WarnTotalHours, WarnNightHours},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewRoadTestAppointment(tt.profile, CreditHours(tt.profile), now)
			if tt.profile.RoadTestDate == nil {
				if got.Date != nil || len(got.Warnings) != 0 {
					t.Fatalf("appointment = %+v, want none", got)
				}
				return
			}
			if DateValue(got.Date) != DateValue(tt.profile.RoadTestDate) || got.DaysLeft != tt.wantDays || got.Passed != tt.wantPassed {
				t.Fatalf("appointment = %+v, want %d days left, passed %v", got, tt.wantDays, tt.wantPassed)
			}
			if kinds := appointmentKinds(got); !reflect.DeepEqual(kinds, tt.wantKinds) {
				t.Fatalf("warnings = %v (%+v), want %v", kinds, got.Warnings, tt.wantKinds)
			}
			for _, want := range tt.wantContains {
				found := false
				for _, warning := range got.Warnings {
					found = found || strings.Contains(warning.Message, want)
				}
				if !found {
					t.Errorf("no warning contains %q: %+v", want, got.Warnings)
				}
			}
		})
	}
}

func TestRoadTestAppointmentWarnsWhenWeeklyCapDelaysHours(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	permit, appointment := dateOn("2026-10-17"), dateOn("2026-12-01")
	// 45 hours logged in one week over the 30 days since the permit is a pace
	// of 1.5 hours a day. Uncapped, the 15 hours still needed take 10 days,
	// well inside the 15 before the appointment; but only 10 hours of that
	// week are credited and later weeks are capped at 10 as well.
	profile := saveHours(Profile{PermitIssueDate: &permit}, 45, 10, dateOn("2026-11-10"))
	profile.RoadTestDate = &appointment
	credit := CreditHours(profile)
	if credit.TotalHours != 10 {
		t.Fatalf("credit = %+v, want the weekly cap to leave 10 total hours", credit)
	}
	got := NewRoadTestAppointment(profile, credit, now)
	// The hold also ends after 2026-12-01.
	want := []AppointmentWarningKind{WarnBeforeEligible, WarnTotalHours}
	if kinds := appointmentKinds(got); !reflect.DeepEqual(kinds, want) {
		t.Fatalf("warnings = %+v, want %v", got.Warnings, want)
	}
}

func TestNewDashboardIncludesAppointment(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	permit, appointment := dateOn("2026-01-15"), dateOn("2026-12-01")
	profile := Profile{PermitIssueDate: &permit, RoadTestDate: &appointment, TotalHours: 60, NightHours: 10}
	dashboard := NewDashboard(NewDriver("aiden"), profile, DefaultRequirements(now), now)
	if dashboard.Appointment.Date == nil || dashboard.Appointment.DaysLeft != 15 {
		t.Fatalf("appointment = %+v", dashboard.Appointment)
	}
}
