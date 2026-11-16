package repository

import (
	"context"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

func TestRoadTestDateMigrationIsAdditive(t *testing.T) {
	pool := testSchemaPool(t)
	migrate(t, pool, 1, 7)
	ctx := context.Background()
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	driver, err := NewPostgresStore(pool).EnsureDriver(ctx, "before-migration", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `update app_profile set total_hours=12, night_hours=2, permit_issue_date='2026-01-15' where driver_id=$1`, driver.ID); err != nil {
		t.Fatal(err)
	}
	migrate(t, pool, 8, 8)
	tracker, err := NewPostgresStore(pool).GetTracker(ctx, driver.ID)
	if err != nil {
		t.Fatal(err)
	}
	profile := tracker.Profile
	if profile.RoadTestDate != nil || profile.TotalHours != 12 || profile.NightHours != 2 || model.DateValue(profile.PermitIssueDate) != "2026-01-15" {
		t.Fatalf("existing profile changed: %+v", profile)
	}
	// An older binary's update ignores the new column and leaves it alone.
	if _, err = pool.Exec(ctx, `update app_profile set road_test_date='2027-01-01' where driver_id=$1`, driver.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `update app_profile set permit_issue_date=$2,total_hours=$3,night_hours=$4,updated_at=$5 where driver_id=$1`, driver.ID, profile.PermitIssueDate, 13.0, 2.0, now); err != nil {
		t.Fatal(err)
	}
	if tracker, err = NewPostgresStore(pool).GetTracker(ctx, driver.ID); err != nil || model.DateValue(tracker.Profile.RoadTestDate) != "2027-01-01" {
		t.Fatalf("old-style update lost the appointment: %+v, %v", tracker.Profile, err)
	}
}

func TestRoadTestDateStorageParity(t *testing.T) {
	factories := map[string]func(*testing.T) Store{
		"memory": func(t *testing.T) Store { return NewMemoryStore(time.Now()) },
		"postgres": func(t *testing.T) Store {
			p := testSchemaPool(t)
			migrate(t, p, 1, SchemaVersion)
			return NewPostgresStore(p)
		},
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			store := factory(t)
			ctx := context.Background()
			now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
			driver, err := store.EnsureDriver(ctx, "appointment-driver", now)
			if err != nil {
				t.Fatal(err)
			}
			other, err := store.EnsureDriver(ctx, "other-driver", now)
			if err != nil {
				t.Fatal(err)
			}
			tracker := func(id int64) model.Tracker {
				t.Helper()
				got, err := store.GetTracker(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			if got := tracker(driver.ID).Profile.RoadTestDate; got != nil {
				t.Fatalf("new driver has appointment %v", got)
			}

			date := time.Date(2027, 3, 4, 0, 0, 0, 0, time.UTC)
			saved, err := store.UpdateProfile(ctx, driver.ID, model.Profile{TotalHours: 12, NightHours: 2, RoadTestDate: &date}, now)
			if err != nil {
				t.Fatal(err)
			}
			if model.DateValue(saved.RoadTestDate) != "2027-03-04" {
				t.Fatalf("saved appointment = %v", saved.RoadTestDate)
			}
			got := tracker(driver.ID).Profile
			if model.DateValue(got.RoadTestDate) != "2027-03-04" || got.TotalHours != 12 {
				t.Fatalf("reloaded profile = %+v", got)
			}
			if tracker(other.ID).Profile.RoadTestDate != nil {
				t.Fatal("appointment leaked to another driver")
			}

			// Callers share no date pointer with the store.
			*got.RoadTestDate = time.Time{}
			*saved.RoadTestDate = time.Time{}
			date = time.Time{}
			if model.DateValue(tracker(driver.ID).Profile.RoadTestDate) != "2027-03-04" {
				t.Fatal("stored appointment changed through a returned or passed pointer")
			}

			// An hours-only change keeps the date the caller sends back, and a
			// nil date clears it.
			kept := tracker(driver.ID).Profile
			kept.TotalHours = 14
			if _, err := store.UpdateProfile(ctx, driver.ID, kept, now.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if model.DateValue(tracker(driver.ID).Profile.RoadTestDate) != "2027-03-04" {
				t.Fatal("appointment lost on an hours save")
			}
			kept.RoadTestDate = nil
			if _, err := store.UpdateProfile(ctx, driver.ID, kept, now.Add(2*time.Hour)); err != nil {
				t.Fatal(err)
			}
			if got := tracker(driver.ID).Profile.RoadTestDate; got != nil {
				t.Fatalf("cleared appointment = %v", got)
			}
		})
	}
}
