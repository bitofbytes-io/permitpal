package repository

import (
	"context"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

// A save whose form omitted the appointment must keep whatever is stored when
// it commits, not the value the handler read earlier.
func TestOmittedRoadTestDateKeepsAppointmentSavedAfterRead(t *testing.T) {
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
			driver, err := store.EnsureDriver(ctx, "racing-driver", now)
			if err != nil {
				t.Fatal(err)
			}
			old := time.Date(2027, 1, 10, 0, 0, 0, 0, time.UTC)
			if _, err = store.UpdateProfile(ctx, driver.ID, model.Profile{TotalHours: 5, NightHours: 1, RoadTestDate: &old}, now); err != nil {
				t.Fatal(err)
			}

			// Request A reads the profile, then request B saves a new appointment.
			staleRead, err := store.GetTracker(ctx, driver.ID)
			if err != nil {
				t.Fatal(err)
			}
			newer := time.Date(2027, 5, 20, 0, 0, 0, 0, time.UTC)
			if _, err = store.UpdateProfile(ctx, driver.ID, model.Profile{TotalHours: 5, NightHours: 1, RoadTestDate: &newer}, now.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}

			// A's form omitted the field, and A writes last.
			stale := staleRead.Profile
			stale.TotalHours = 8
			stale.KeepRoadTestDate = true
			saved, err := store.UpdateProfile(ctx, driver.ID, stale, now.Add(2*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if got := model.DateValue(saved.RoadTestDate); got != "2027-05-20" {
				t.Fatalf("returned appointment = %q, want 2027-05-20", got)
			}
			tracker, err := store.GetTracker(ctx, driver.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got := model.DateValue(tracker.Profile.RoadTestDate); got != "2027-05-20" {
				t.Fatalf("stored appointment = %q, want 2027-05-20", got)
			}
			if tracker.Profile.TotalHours != 8 {
				t.Fatalf("hours not saved: %+v", tracker.Profile)
			}

			// A present-but-empty field still clears it.
			cleared := tracker.Profile
			cleared.RoadTestDate = nil
			if _, err = store.UpdateProfile(ctx, driver.ID, cleared, now.Add(3*time.Minute)); err != nil {
				t.Fatal(err)
			}
			if tracker, err = store.GetTracker(ctx, driver.ID); err != nil || tracker.Profile.RoadTestDate != nil {
				t.Fatalf("appointment not cleared: %+v, %v", tracker.Profile, err)
			}
		})
	}
}
