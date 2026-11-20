package handler

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

func TestProfileValidation(t *testing.T) {
	forEachBackend(t, func(t *testing.T, backend string) {
		// "now" is 2026-05-01 in the app time zone.
		f := newFormFixture(t, backend, time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local))
		profileForm := func(total, night, date string) url.Values {
			return url.Values{"total_hours": {total}, "night_hours": {night}, "permit_issue_date": {date}}
		}
		rejected := map[string]struct {
			form url.Values
			want string
		}{
			"night-over-total":   {url.Values{"total_hours": {"4"}, "night_hours": {"4.5"}}, "Night hours cannot be more than total hours"},
			"future-permit-date": {profileForm("4", "1", "2026-05-02"), "Permit issue date cannot be in the future"},
		}
		for _, date := range []string{"not-a-date", "2026-02-30", "2026-13-01", "2026-01-15T12:00:00Z"} {
			rejected["invalid-profile-date/"+date] = struct {
				form url.Values
				want string
			}{profileForm("12.5", "2.0", date), "Permit issue date"}
		}
		for _, field := range []string{"total_hours", "night_hours"} {
			for _, value := range []string{"NaN", "Inf", "+Inf", "-Inf", "Infinity", "1.23", "-1", "1000.1"} {
				form := profileForm("12.5", "2.0", "2026-01-15")
				form.Set(field, value)
				rejected[field+"/"+value] = struct {
					form url.Values
					want string
				}{form, ""}
			}
		}
		for name, test := range rejected {
			t.Run(name, func(t *testing.T) { f.requireRejected(t, "/profile", test.form, test.want) })
		}
		t.Run("today-and-equal-hours-are-allowed", func(t *testing.T) {
			f.requireOK(t, "/profile", profileForm("4", "4", "2026-05-01"))
		})
		for _, date := range []string{"2026-01-15", "", "  "} {
			t.Run("valid-or-cleared-date/"+date, func(t *testing.T) {
				f.requireOK(t, "/profile", profileForm("12.5", "2.0", date))
				profile := f.load(t).Profile
				if model.DateValue(profile.PermitIssueDate) != strings.TrimSpace(date) || profile.TotalHours != 12.5 || profile.NightHours != 2 {
					t.Fatalf("unexpected saved profile: %+v", profile)
				}
			})
		}
	})
}
