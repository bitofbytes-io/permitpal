package repository

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

func TestMemoryDriversAreIsolatedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store := NewMemoryStore(now)
	aiden, err := store.EnsureDriver(ctx, "aiden", now)
	if err != nil {
		t.Fatal(err)
	}
	caleb, err := store.EnsureDriver(ctx, "caleb", now)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := store.GetTracker(ctx, caleb.ID)
	again, _ := store.EnsureDriver(ctx, "aiden", now)
	if again != aiden {
		t.Fatal("EnsureDriver changed driver")
	}
	dash, _ := store.GetTracker(ctx, aiden.ID)
	if len(dash.Requirements) != 17 || dash.Profile.TotalHours != 0 || dash.Profile.PermitIssueDate != nil {
		t.Fatalf("new dashboard: %+v", dash)
	}
	date := now
	profile := model.Profile{TotalHours: 12, NightHours: 2, PermitIssueDate: &date}
	saved, err := store.UpdateProfile(ctx, aiden.ID, profile)
	if err != nil {
		t.Fatal(err)
	}
	req := dash.Requirements[0]
	req.Rating = model.RatingBad
	req.RatedOn = &date
	req.Notes = "Practice"
	savedReq, err := store.UpdateRequirement(ctx, aiden.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	*saved.PermitIssueDate = time.Time{}
	*savedReq.RatedOn = time.Time{}
	date = time.Time{}
	dash, _ = store.GetTracker(ctx, aiden.ID)
	if dash.Profile.PermitIssueDate.IsZero() || dash.Requirements[0].RatedOn.IsZero() {
		t.Fatal("write results or arguments share date pointers")
	}
	*dash.Profile.PermitIssueDate = time.Time{}
	*dash.Requirements[0].RatedOn = time.Time{}
	next, _ := store.GetTracker(ctx, aiden.ID)
	if next.Profile.PermitIssueDate.IsZero() || next.Requirements[0].RatedOn.IsZero() {
		t.Fatal("dashboard shares date pointers")
	}
	after, _ := store.GetTracker(ctx, caleb.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("Aiden changed Caleb data")
	}
	again, _ = store.EnsureDriver(ctx, "aiden", now)
	next, _ = store.GetTracker(ctx, again.ID)
	if next.Profile.TotalHours != 12 || next.Requirements[0].Notes != "Practice" {
		t.Fatal("login reset saved data")
	}
	if _, err := store.DriverByUsername(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("missing driver: %v", err)
	}
}
