package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

func TestProgressPanelShowsMilestoneBadgesWithDates(t *testing.T) {
	on := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	milestones := []model.Milestone{
		{Key: "total-10", Label: "10 total hours", Reached: true, On: &on},
		{Key: "total-30", Label: "30 total hours", Reached: true},
		{Key: "total-45", Label: "45 total hours"},
	}
	var buf bytes.Buffer
	if err := ProgressPanel(model.Dashboard{Milestones: milestones}).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	for _, want := range []string{
		`datetime="2026-03-04"`, "Mar 4, 2026", "Before dates were recorded", "Not yet",
		"milestone milestone--reached", "45 total hours",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("panel missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "milestone-banner") {
		t.Error("a plain page load must not show the celebration banner")
	}
}

func TestProfileUpdateShowsCelebrationBannerForEarnedMilestones(t *testing.T) {
	var buf bytes.Buffer
	earned := []model.Milestone{{Label: "30 total hours", Reached: true}, {Label: "Night hours complete", Reached: true}}
	if err := ProfileUpdate(model.Dashboard{Celebrations: earned}).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	if !strings.Contains(body, `class="milestone-banner" role="status"`) || !strings.Contains(body, "30 total hours, Night hours complete") {
		t.Errorf("update missing celebration banner: %s", body)
	}
}
