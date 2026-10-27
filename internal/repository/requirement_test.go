package repository

import (
	"context"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

func TestRequirementMasteryTransitions(t *testing.T) {
	factories := map[string]func(*testing.T) Store{
		"memory": func(t *testing.T) Store { return NewMemoryStore(time.Now()) },
		"postgres": func(t *testing.T) Store {
			pool := testSchemaPool(t)
			migrate(t, pool, 1, SchemaVersion)
			return NewPostgresStore(pool)
		},
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			store := factory(t)
			ctx := context.Background()
			today := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
			driver, err := store.EnsureDriver(ctx, "aiden", today)
			if err != nil {
				t.Fatal(err)
			}
			load := func() model.Requirement {
				t.Helper()
				tracker, err := store.GetTracker(ctx, driver.ID)
				if err != nil {
					t.Fatal(err)
				}
				req, ok := model.RequirementByKey(tracker.Requirements, "quick-stop")
				if !ok {
					t.Fatal("quick-stop missing")
				}
				return req
			}
			save := func(req model.Requirement) model.Requirement {
				t.Helper()
				saved, err := store.UpdateRequirement(ctx, driver.ID, req)
				if err != nil {
					t.Fatal(err)
				}
				return saved
			}

			mastered := save(model.ApplyRequirementUpdate(load(), model.RatingGood, nil, "Signed off", today))
			if mastered.Rating != model.RatingGood || model.DateValue(mastered.RatedOn) != "2026-10-27" || mastered.Notes != "Signed off" {
				t.Fatalf("saved mastered: %+v", mastered)
			}
			if got := load(); got.Rating != model.RatingGood || model.DateValue(got.RatedOn) != "2026-10-27" || got.Notes != "Signed off" {
				t.Fatalf("stored mastered: %+v", got)
			}

			// Submitting the sign-off date again with a practice rating clears it.
			practiced := save(model.ApplyRequirementUpdate(load(), model.RatingFair, load().RatedOn, "Needs practice", today))
			if practiced.Rating != model.RatingFair || practiced.RatedOn != nil || practiced.Notes != "Needs practice" {
				t.Fatalf("saved practice: %+v", practiced)
			}
			if got := load(); got.Rating != model.RatingFair || got.RatedOn != nil || got.Notes != "Needs practice" {
				t.Fatalf("stored practice: %+v", got)
			}

			edited := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
			mastered = save(model.ApplyRequirementUpdate(load(), model.RatingGood, &edited, "Needs practice", today))
			if model.DateValue(mastered.RatedOn) != "2026-08-15" {
				t.Fatalf("edited mastered date: %+v", mastered)
			}
			cleared := save(model.ApplyRequirementUpdate(load(), model.RatingNotRated, &edited, "Needs practice", today))
			if cleared.Rating != model.RatingNotRated || cleared.RatedOn != nil || cleared.Notes != "Needs practice" {
				t.Fatalf("saved clear: %+v", cleared)
			}
			if got := load(); got.Rating != model.RatingNotRated || got.RatedOn != nil || got.Notes != "Needs practice" {
				t.Fatalf("stored clear: %+v", got)
			}
		})
	}
}
