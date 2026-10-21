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
}
