package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

func TestProgressPanelNamesReadyConstraint(t *testing.T) {
	date := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		estimate model.ReadyEstimate
		want     []string
	}{
		{model.ReadyEstimate{Date: &date, WaitingOn: model.ConstraintPermitHold}, []string{"Hold ends", "October 1, 2026", "Waiting on 9-month hold"}},
		{model.ReadyEstimate{Date: &date, WaitingOn: model.ConstraintNightHours}, []string{"On pace for", "Waiting on night hours"}},
		{model.ReadyEstimate{Date: &date, WaitingOn: model.ConstraintWeeklyCap}, []string{"Waiting on 10-hour weekly cap"}},
		{model.ReadyEstimate{Date: &date, WaitingOn: model.ConstraintTotalHours}, []string{"On pace for", "Waiting on total hours"}},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		if err := ProgressPanel(model.Dashboard{ReadyEstimate: tt.estimate}).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		for _, want := range tt.want {
			if !strings.Contains(buf.String(), want) {
				t.Errorf("%s: panel missing %q", tt.estimate.WaitingOn, want)
			}
		}
	}

	var buf bytes.Buffer
	if err := ProgressPanel(model.Dashboard{ReadyEstimate: model.ReadyEstimate{Hint: "Add hours to estimate"}}).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "estimate-note") {
		t.Error("hint-only estimate should not show a constraint note")
	}
}

func TestWeekChartFlagsCappedWeeksAndMatchesTotals(t *testing.T) {
	start := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	chart := model.WeekCreditChart{
		Weeks: []model.WeekCreditBar{
			{Starts: start, LoggedTotal: 16, LoggedNight: 4, CreditedTotal: 10, CreditedNight: 4, Capped: true},
			{Starts: start.AddDate(0, 0, 7), LoggedTotal: 3, LoggedNight: 1, CreditedTotal: 3, CreditedNight: 1},
		},
		WindowTotal:  13,
		WindowNight:  5,
		UndatedTotal: 4,
		UndatedNight: 1,
		TotalHours:   17,
		NightHours:   6,
	}
	var buf bytes.Buffer
	if err := WeekCreditChart(chart).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	for _, want := range []string{
		"Hours by entry week",
		"week-flag",
		"week-label week-label--capped",
		"Capped: week of Aug 3, 2026 (lost 6.0).",
		"These 2 weeks credit 13.0 total and 5.0 night.",
		"Undated opening hours add 4.0 total and 1.0 night.",
		"Together this is 17.0 total and 6.0 night, the same weekly credited total shown above.",
		">Aug</text>",
		"<td>16.0</td>",
		"<td>Yes</td>",
		`<details class="week-values-disclosure">`,
		"<summary>Exact hours by week</summary>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("chart missing %q\n%s", want, body)
		}
	}
	if strings.Count(body, "<caption>Logged and credited hours by entry week</caption>") != 2 {
		t.Fatalf("want the disclosure table and the screen-reader table, html:\n%s", body)
	}
	disclosure := strings.Index(body, `<details class="week-values-disclosure">`)
	srOnly := strings.Index(body, `<div class="sr-only">`)
	closeDetails := strings.Index(body, "</details>")
	if disclosure < 0 || srOnly < 0 || closeDetails < 0 || !(disclosure < closeDetails && closeDetails < srOnly) {
		t.Fatalf("screen-reader table must stay outside the disclosure, html:\n%s", body)
	}

	var panel bytes.Buffer
	dashboard := model.Dashboard{HourCredit: model.HourCredit{TotalHours: chart.TotalHours, NightHours: chart.NightHours}, WeekChart: chart}
	if err := ProgressPanel(dashboard).Render(context.Background(), &panel); err != nil {
		t.Fatal(err)
	}
	panelHTML := panel.String()
	if !strings.Contains(panelHTML, "Weekly credited hours: 17.0 total, including 6.0 night.") || !strings.Contains(panelHTML, "Together this is 17.0 total and 6.0 night") {
		t.Fatalf("panel totals diverged: %s", panelHTML)
	}
}

func TestLastDriveNudgeHighlightsGapsAfterSevenDays(t *testing.T) {
	on := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		nudge model.LastDrive
		want  []string
		miss  []string
	}{
		{
			name:  "no dated entries",
			nudge: model.LastDrive{RemainingHours: 10},
			want:  []string{"Days since last drive", "No dated driving entries yet.", "10.0 hours of this week's credit remain."},
			miss:  []string{"drive-nudge--stale", "save date"},
		},
		{
			name:  "quiet week",
			nudge: model.LastDrive{Dated: true, Days: 7, RecordedOn: &on, RemainingHours: 3.5},
			want:  []string{`class="drive-nudge"`, "7 days", "since the last dated entry (Oct 13, 2026)", "3.5 hours of this week's credit remain.", "save date, not the actual drive date"},
			miss:  []string{"drive-nudge--stale", "longer than 7 days"},
		},
		{
			name:  "stale gap",
			nudge: model.LastDrive{Dated: true, Days: 14, RecordedOn: &on, RemainingHours: 10, Stale: true},
			want:  []string{"drive-nudge drive-nudge--stale", "14 days", "longer than 7 days", "10.0 hours of this week's credit remain."},
		},
		{
			name:  "logged today",
			nudge: model.LastDrive{Dated: true, RecordedOn: &on, RemainingHours: 0},
			want:  []string{"Last dated entry was today", "This week's 10-hour credit is used up."},
			miss:  []string{"drive-nudge--stale"},
		},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		if err := ProgressPanel(model.Dashboard{LastDrive: tt.nudge}).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		body := buf.String()
		for _, want := range tt.want {
			if !strings.Contains(body, want) {
				t.Errorf("%s: panel missing %q", tt.name, want)
			}
		}
		for _, miss := range tt.miss {
			if strings.Contains(body, miss) {
				t.Errorf("%s: panel unexpectedly contains %q", tt.name, miss)
			}
		}
	}
}

func TestWhatsLeftCountsDownPermitHold(t *testing.T) {
	ends := time.Date(2026, 12, 2, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		hold model.PermitHold
		want []string
	}{
		{"running", model.PermitHold{Ends: &ends, DaysLeft: 45}, []string{"<strong>45</strong> days to go", "9-month hold ends Dec 2, 2026", "must be Dec 2, 2026 or later"}},
		{"last day", model.PermitHold{Ends: &ends, DaysLeft: 1}, []string{"<strong>1</strong> day to go"}},
		{"over", model.PermitHold{Ends: &ends}, []string{"Hold complete", "9-month hold ended Dec 2, 2026"}},
		{"no issue date", model.PermitHold{}, []string{"Add your permit issue date to count down the 9-month hold."}},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		if err := WhatsLeft(model.Dashboard{PermitHold: tt.hold}, false).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		for _, want := range append(tt.want, "6 months with no violations", "Proof of insurance", "Schedule the road test") {
			if !strings.Contains(buf.String(), want) {
				t.Errorf("%s: card missing %q", tt.name, want)
			}
		}
		if strings.Contains(buf.String(), "hx-swap-oob") {
			t.Errorf("%s: in-page card should not swap out of band", tt.name)
		}
	}
}

func TestProfileUpdateRefreshesCountdown(t *testing.T) {
	var buf bytes.Buffer
	if err := ProfileUpdate(model.Dashboard{}).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	if !strings.Contains(body, `id="progress-panel"`) || !strings.Contains(body, "Progress saved") {
		t.Errorf("update missing saved progress panel: %s", body)
	}
	if !strings.Contains(body, `id="whats-left"`) || !strings.Contains(body, `hx-swap-oob="outerHTML"`) {
		t.Errorf("update missing out-of-band countdown: %s", body)
	}
	if !strings.Contains(body, `id="road-test-readiness"`) || strings.Count(body, `hx-swap-oob="outerHTML"`) < 2 {
		t.Errorf("update missing out-of-band readiness card: %s", body)
	}
}

func TestRoadTestReadinessCardPartialAndReady(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	requirements := model.DefaultRequirements(now)
	for i := range requirements {
		requirements[i].Rating = model.RatingGood
	}
	for key, rating := range map[string]model.RequirementRating{
		"turn-about": model.RatingNotRated,
		"backing":    model.RatingFair,
		"left-turns": model.RatingBad,
		"following":  model.RatingNotRated,
	} {
		for i := range requirements {
			if requirements[i].Key == key {
				requirements[i].Rating = rating
			}
		}
	}
	partial := model.NewDashboard(model.NewDriver("aiden"), model.Profile{TotalHours: 42, NightHours: 6.5}, requirements, now)
	var buf bytes.Buffer
	if err := RoadTestReadinessCard(partial, false).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	for _, want := range []string{
		`id="road-test-readiness"`,
		"Road test readiness",
		`href="#total_hours"`,
		"18.0 total hours left",
		`href="#night_hours"`,
		"3.5 night hours left",
		">Hours<",
		">Maneuvers<",
		`href="#requirement-turn-about"`,
		"Turn about",
		`href="#requirement-backing"`,
		">Intersections<",
		`href="#requirement-left-turns"`,
		">General driving<",
		`href="#requirement-following"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("partial card missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "hx-swap-oob") || strings.Contains(body, "readiness-card--ready") || strings.Contains(body, ">Ready<") || strings.Contains(body, "Quick stop") {
		t.Fatalf("partial card should stay not-ready and omit mastered skills:\n%s", body)
	}

	readyReqs := model.DefaultRequirements(now)
	for i := range readyReqs {
		readyReqs[i].Rating = model.RatingGood
	}
	ready := model.NewDashboard(model.NewDriver("aiden"), model.Profile{TotalHours: 60, NightHours: 10}, readyReqs, now)
	buf.Reset()
	if err := RoadTestReadinessCard(ready, true).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	body = buf.String()
	for _, want := range []string{`id="road-test-readiness"`, `hx-swap-oob="outerHTML"`, "readiness-card--ready", ">Ready<", "Credited hours and every skill are met."} {
		if !strings.Contains(body, want) {
			t.Errorf("ready card missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "hours left") || strings.Contains(body, "requirement-") || strings.Contains(body, ">Maneuvers<") {
		t.Fatalf("ready card still lists work:\n%s", body)
	}
}
