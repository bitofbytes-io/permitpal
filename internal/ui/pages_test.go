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
		{model.ReadyEstimate{Date: &date, WaitingOn: model.ConstraintPermitHold}, []string{"Eligible on", "October 1, 2026", "Waiting on 9-month hold"}},
		{model.ReadyEstimate{Date: &date, WaitingOn: model.ConstraintNightHours}, []string{"On pace for", "Waiting on night hours"}},
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
