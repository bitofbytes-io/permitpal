package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

func renderRow(t *testing.T, req model.Requirement) string {
	t.Helper()
	var buf bytes.Buffer
	if err := RequirementRow(req).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestMasteredRowStatusIsTheDate(t *testing.T) {
	date := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	body := renderRow(t, model.Requirement{
		Key:         "quick-stop",
		Title:       "Quick stop",
		Description: "Stop quickly.",
		Rating:      model.RatingGood,
		RatedOn:     &date,
		Notes:       "Signed off after a clean stop.",
		SortOrder:   1,
	})
	for _, want := range []string{
		"Mastered Oct 27, 2026",
		`data-mastered="2026-10-27"`,
		`aria-label="Mastered Oct 27, 2026"`,
		"Signed off after a clean stop.",
		`data-order="1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("mastered row missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "Last rated") {
		t.Fatalf("mastered row shows a separate last-rated label:\n%s", body)
	}
}

func TestPracticeRowKeepsLastRatedDate(t *testing.T) {
	date := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	body := renderRow(t, model.Requirement{
		Key: "use-of-lane", Title: "Use of lane", Rating: model.RatingFair, RatedOn: &date, Notes: "Drifts right", SortOrder: 12,
	})
	if strings.Contains(body, "Mastered") || strings.Contains(body, "data-mastered") {
		t.Fatalf("practice row shows a mastered status:\n%s", body)
	}
	for _, want := range []string{"Last rated", "Drifts right", `value="2026-10-20"`, `data-order="12"`} {
		if !strings.Contains(body, want) {
			t.Errorf("practice row missing %q\n%s", want, body)
		}
	}
}

func TestChecklistOffersRecentlyMasteredView(t *testing.T) {
	var buf bytes.Buffer
	dashboard := model.NewDashboard(model.Driver{DisplayName: "Aiden"}, model.Profile{}, model.DefaultRequirements(time.Now()), time.Now())
	if err := DashboardPage(dashboard).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	for _, want := range []string{
		`data-checklist-view="sheet"`,
		`data-checklist-view="recent"`,
		"Recently mastered",
		"All skills",
		"No skills are mastered yet.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("checklist missing %q", want)
		}
	}
	if strings.Contains(body, "data-mastered=") {
		t.Error("an unmastered checklist rendered a mastered date")
	}
}
