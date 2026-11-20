package handler

import (
	"context"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

func TestConcurrentEnsureDriver(t *testing.T) {
	forEachBackend(t, func(t *testing.T, backend string) {
		now := time.Now()
		store := validationStore(t, backend, now)
		ctx := context.Background()
		type result struct {
			driver model.Driver
			err    error
		}
		results := make(chan result, 8)
		for i := 0; i < 8; i++ {
			go func() { driver, err := store.EnsureDriver(ctx, "aiden", now); results <- result{driver, err} }()
		}
		var first model.Driver
		for i := 0; i < 8; i++ {
			result := <-results
			if result.err != nil {
				t.Fatal(result.err)
			}
			if i == 0 {
				first = result.driver
			}
			if result.driver != first {
				t.Fatal("concurrent login created different drivers")
			}
		}
		dash, err := store.GetTracker(ctx, first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(dash.Requirements) != 17 || dash.Profile.TotalHours != 0 {
			t.Fatal("concurrent login did not create one complete fresh tracker")
		}
	})
}

func TestEndSessionsAdvancesTheGenerationOnce(t *testing.T) {
	forEachBackend(t, func(t *testing.T, backend string) {
		ctx := context.Background()
		now := time.Now()
		store := validationStore(t, backend, now)
		created, err := store.EnsureDriver(ctx, "aiden", now)
		if err != nil {
			t.Fatal(err)
		}
		if created.SessionGeneration != 0 {
			t.Fatalf("new driver generation = %d", created.SessionGeneration)
		}
		for _, generation := range []int64{0, 0, 5} {
			if err := store.EndSessions(ctx, "aiden", generation); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.EndSessions(ctx, "missing", 0); err != nil {
			t.Fatal(err)
		}
		byName, err := store.DriverByUsername(ctx, "aiden")
		if err != nil {
			t.Fatal(err)
		}
		again, err := store.EnsureDriver(ctx, "aiden", now)
		if err != nil {
			t.Fatal(err)
		}
		if byName.SessionGeneration != 1 || again.SessionGeneration != 1 {
			t.Fatalf("generation = %d / %d, want 1 after one matching logout", byName.SessionGeneration, again.SessionGeneration)
		}
	})
}
