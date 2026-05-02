package repository

import (
	"context"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

func TestMemoryStoreSeedsDashboardAndUpdatesRequirement(t *testing.T) {
	store := NewMemoryStore(time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local))

	dashboard, err := store.GetDashboard(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if len(dashboard.Requirements) != 13 {
		t.Fatalf("len(requirements) = %d, want 13", len(dashboard.Requirements))
	}

	req, ok := model.RequirementByKey(dashboard.Requirements, "lane-changes")
	if !ok {
		t.Fatal("missing lane-changes requirement")
	}
	req.Status = model.StatusMastered
	mastered := time.Date(2026, 5, 2, 0, 0, 0, 0, time.Local)
	req.MasteredDate = &mastered
	req.Notes = "Clean mirror checks."

	updated, err := store.UpdateRequirement(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != model.StatusMastered || updated.MasteredDate == nil {
		t.Fatalf("updated requirement = %#v, want mastered with date", updated)
	}
}

func TestMemoryStoreUpdatesProfile(t *testing.T) {
	store := NewMemoryStore(time.Now())
	profile := model.Profile{TotalHours: 42, NightHours: 8}

	updated, err := store.UpdateProfile(context.Background(), profile)
	if err != nil {
		t.Fatal(err)
	}
	if updated.TotalHours != 42 || updated.NightHours != 8 {
		t.Fatalf("updated profile = %#v", updated)
	}
}

func TestMemoryStoreDashboardDoesNotShareDatePointers(t *testing.T) {
	store := NewMemoryStore(time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local))
	dashboard, err := store.GetDashboard(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}

	*dashboard.Profile.PermitIssueDate = time.Date(1999, 1, 1, 0, 0, 0, 0, time.Local)
	for idx := range dashboard.Requirements {
		if dashboard.Requirements[idx].MasteredDate != nil {
			*dashboard.Requirements[idx].MasteredDate = time.Date(1999, 1, 1, 0, 0, 0, 0, time.Local)
			break
		}
	}

	next, err := store.GetDashboard(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if next.Profile.PermitIssueDate.Year() == 1999 {
		t.Fatal("profile permit issue date shared pointer state")
	}
	for _, req := range next.Requirements {
		if req.MasteredDate != nil && req.MasteredDate.Year() == 1999 {
			t.Fatal("requirement mastered date shared pointer state")
		}
	}
}
