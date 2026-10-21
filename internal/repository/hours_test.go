package repository

import (
	"context"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

func TestHourEntriesStorageParity(t *testing.T) {
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
			loc, _ := time.LoadLocation("America/New_York")
			sunday := time.Date(2027, 1, 3, 23, 30, 0, 0, loc)
			driver, err := store.EnsureDriver(ctx, "entry-driver", sunday)
			if err != nil {
				t.Fatal(err)
			}
			other, err := store.EnsureDriver(ctx, "other-driver", sunday)
			if err != nil {
				t.Fatal(err)
			}
			save := func(total, night float64, at time.Time) {
				t.Helper()
				_, err := store.UpdateProfile(ctx, driver.ID, model.Profile{TotalHours: total, NightHours: night}, at)
				if err != nil {
					t.Fatal(err)
				}
			}
			tracker := func() model.Tracker {
				t.Helper()
				got, err := store.GetTracker(ctx, driver.ID)
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			save(15, 2, sunday)
			first := tracker()
			if len(first.Profile.HourEntries) != 1 || model.CreditHours(first.Profile).TotalHours != 10 {
				t.Fatal(first.Profile)
			}
			if entry := first.Profile.HourEntries[0]; !entry.RecordedAt.Equal(sunday) || model.DateValue(&entry.RecordedOn) != "2027-01-03" {
				t.Fatal(entry)
			}
			// Simultaneous duplicate requests compare against the locked stored value.
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			for range 8 {
				wg.Go(func() {
					_, err := store.UpdateProfile(ctx, driver.ID, model.Profile{TotalHours: 15, NightHours: 2}, sunday)
					errs <- err
				})
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := tracker(); len(got.Profile.HourEntries) != 1 {
				t.Fatalf("duplicate submissions journaled: %+v", got.Profile.HourEntries)
			}
			// A caller cannot mutate the memory store's journal through returned data.
			first.Profile.HourEntries[0].TotalTenths = 600
			if tracker().Profile.HourEntries[0].TotalTenths != 150 {
				t.Fatal("shared journal slice")
			}
			save(12, 2, sunday)
			if c := model.CreditHours(tracker().Profile); c.TotalHours != 10 {
				t.Fatal(c)
			}
			save(8, 2, sunday)
			if c := model.CreditHours(tracker().Profile); c.TotalHours != 8 {
				t.Fatal(c)
			}
			save(18, 3, sunday.Add(time.Hour))
			p := tracker().Profile
			c := model.CreditHours(p)
			if c.TotalHours != 18 || c.NightHours != 3 || len(c.Weeks) != 2 {
				t.Fatalf("Monday credit=%+v", c)
			}
			permit := sunday.AddDate(-1, 0, 0)
			p.PermitIssueDate = &permit
			if _, err := store.UpdateProfile(ctx, driver.ID, p, sunday.Add(2*time.Hour)); err != nil {
				t.Fatal(err)
			}
			after := tracker()
			if len(after.Profile.HourEntries) != 4 {
				t.Fatalf("permit-only edit journaled: %+v", after.Profile.HourEntries)
			}
			untouched, err := store.GetTracker(ctx, other.ID)
			if err != nil || untouched.Profile.TotalHours != 0 || len(untouched.Profile.HourEntries) != 0 {
				t.Fatalf("other driver changed: %+v %v", untouched, err)
			}
			// Fresh store instance retains every entry in Postgres; another login never resets it.
			store.EnsureDriver(ctx, driver.Username, sunday)
			if !reflect.DeepEqual(after.Profile.HourEntries, tracker().Profile.HourEntries) {
				t.Fatal("login changed journal")
			}
			more, err := store.EnsureDriver(ctx, "more-hours", sunday)
			if err != nil {
				t.Fatal(err)
			}
			saveMore := func(total, night float64, at time.Time) {
				t.Helper()
				if _, err := store.UpdateProfile(ctx, more.ID, model.Profile{TotalHours: total, NightHours: night}, at); err != nil {
					t.Fatal(err)
				}
			}
			saveMore(60, 20, sunday)
			saveMore(61, 21, sunday.AddDate(0, 0, 1))
			advanced, err := store.GetTracker(ctx, more.ID)
			if err != nil {
				t.Fatal(err)
			}
			if credit := model.CreditHours(advanced.Profile); credit.TotalHours != 11 || credit.NightHours != 11 {
				t.Fatalf("new-week hours=%+v", credit)
			}
			saveMore(model.MaxRecordedHours, model.MaxRecordedHours, sunday.AddDate(0, 0, 2))
			for _, bad := range []float64{model.MaxRecordedHours + 0.1, math.Inf(1), math.NaN()} {
				if _, err := store.UpdateProfile(ctx, more.ID, model.Profile{TotalHours: bad, NightHours: 20}, sunday); err == nil {
					t.Fatalf("total %v accepted", bad)
				}
				if _, err := store.UpdateProfile(ctx, more.ID, model.Profile{TotalHours: model.MaxRecordedHours, NightHours: bad}, sunday); err == nil {
					t.Fatalf("night %v accepted", bad)
				}
			}
			bounded, err := store.GetTracker(ctx, more.ID)
			if err != nil {
				t.Fatal(err)
			}
			if bounded.Profile.TotalHours != model.MaxRecordedHours || bounded.Profile.NightHours != model.MaxRecordedHours || len(bounded.Profile.HourEntries) != 3 {
				t.Fatalf("invalid input changed journal: %+v", bounded.Profile)
			}

		})
	}
}

func TestHourEntryMigrationPreservesLegacyAndReconcilesOldWriter(t *testing.T) {
	pool := testSchemaPool(t)
	migrate(t, pool, 1, 6)
	ctx := context.Background()
	store := NewPostgresStore(pool)
	driver, err := store.DriverByUsername(ctx, "caleb")
	if err != nil {
		t.Fatal(err)
	}
	// Such a row was deliberately preserved by migration005's NOT VALID rule.
	if _, err = pool.Exec(ctx, `alter table app_profile drop constraint app_profile_night_hours_within_total; update app_profile set total_hours=3,night_hours=5; alter table app_profile add constraint app_profile_night_hours_within_total check (night_hours<=total_hours) not valid`); err != nil {
		t.Fatal(err)
	}
	migrate(t, pool, 7, 7)
	tracker, err := store.GetTracker(ctx, driver.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tracker.Profile.TotalHours != 3 || tracker.Profile.NightHours != 5 || len(tracker.Profile.HourEntries) != 1 || !tracker.Profile.HourEntries[0].RecordedOn.IsZero() {
		t.Fatalf("legacy changed: %+v", tracker.Profile)
	}
	// A corrected old-binary save adds hours without a journal write.
	if _, err = pool.Exec(ctx, `update app_profile set total_hours=12,night_hours=5 where driver_id=$1`, driver.ID); err != nil {
		t.Fatal(err)
	}
	tracker, err = store.GetTracker(ctx, driver.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c := model.CreditHours(tracker.Profile); c.TotalHours != 12 || c.UndatedTotal != 12 {
		t.Fatalf("old write assigned a date: %+v", c)
	}
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	p := tracker.Profile
	p.TotalHours = 15
	if _, err = store.UpdateProfile(ctx, driver.ID, p, at); err != nil {
		t.Fatal(err)
	}
	tracker, err = store.GetTracker(ctx, driver.ID)
	if err != nil {
		t.Fatal(err)
	}
	entries := tracker.Profile.HourEntries
	if len(entries) != 3 || !entries[1].RecordedOn.IsZero() || entries[2].RecordedOn.IsZero() {
		t.Fatalf("entries=%+v", entries)
	}
	if c := model.CreditHours(tracker.Profile); c.TotalHours != 15 || c.UndatedTotal != 12 {
		t.Fatal(c)
	}
	// Bad writes roll back both profile and entry; migration rollback preserves totals.
	if _, err = store.UpdateProfile(ctx, driver.ID, model.Profile{TotalHours: 10, NightHours: 11}, at); err == nil {
		t.Fatal("invalid write accepted")
	}
	got, err := store.GetTracker(ctx, driver.ID)
	if err != nil || !reflect.DeepEqual(got.Profile, tracker.Profile) {
		t.Fatalf("partial write: %+v, %v", got.Profile, err)
	}
}

func TestHourEntryMigrationRollbackKeepsCumulativeTotals(t *testing.T) {
	pool := testSchemaPool(t)
	migrate(t, pool, 1, SchemaVersion)
	ctx := context.Background()
	store := NewPostgresStore(pool)
	driver, err := store.EnsureDriver(ctx, "rollback-driver", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateProfile(ctx, driver.ID, model.Profile{TotalHours: 61, NightHours: 21}, time.Now()); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../migrations/007_hour_entries.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, down, _ := strings.Cut(string(source), "-- +goose Down")
	if _, err = pool.Exec(ctx, down); err != nil {
		t.Fatal(err)
	}
	var total, night float64
	if err = pool.QueryRow(ctx, `select total_hours,night_hours from app_profile where driver_id=$1`, driver.ID).Scan(&total, &night); err != nil || total != 61 || night != 21 {
		t.Fatalf("rollback totals=%v/%v: %v", total, night, err)
	}
	migrate(t, pool, 7, 7)
	tracker, err := store.GetTracker(ctx, driver.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c := model.CreditHours(tracker.Profile); c.UndatedTotal != 61 || c.UndatedNight != 21 {
		t.Fatalf("reinstalled history invented dates: %+v", c)
	}
}

func TestProfileHoursRejectUnsupportedPrecision(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var store Store = NewMemoryStore(time.Now())
			if backend == "postgres" {
				pool := testSchemaPool(t)
				migrate(t, pool, 1, SchemaVersion)
				store = NewPostgresStore(pool)
			}
			ctx := context.Background()
			at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
			driver, err := store.EnsureDriver(ctx, "precision-driver", at)
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.GetTracker(ctx, driver.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, profile := range []model.Profile{
				{TotalHours: 0.01},
				{TotalHours: 1.01, NightHours: 0.1},
				{TotalHours: 1, NightHours: 0.01},
				{TotalHours: 1, NightHours: 0.15},
				{TotalHours: math.Nextafter(0.1, 1)},
				{TotalHours: model.MaxRecordedHours, NightHours: 999.99},
			} {
				if _, err := store.UpdateProfile(ctx, driver.ID, profile, at); err == nil {
					t.Fatalf("unsupported precision accepted: %+v", profile)
				}
				after, err := store.GetTracker(ctx, driver.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("unsupported precision changed tracker: %+v", after)
				}
			}
			saved, err := store.UpdateProfile(ctx, driver.ID, model.Profile{TotalHours: 0.1, NightHours: 0.1}, at)
			if err != nil {
				t.Fatal(err)
			}
			if saved.TotalHours != 0.1 || saved.NightHours != 0.1 || len(saved.HourEntries) != 1 || saved.HourEntries[0].TotalTenths != 1 || saved.HourEntries[0].NightTenths != 1 {
				t.Fatalf("valid tenth did not persist: %+v", saved)
			}
			after, err := store.GetTracker(ctx, driver.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Profile.TotalHours != 0.1 || after.Profile.NightHours != 0.1 || len(after.Profile.HourEntries) != 1 {
				t.Fatalf("valid tenth not stored: %+v", after.Profile)
			}
			if _, err := store.UpdateProfile(ctx, driver.ID, model.Profile{TotalHours: 0.11, NightHours: 0.1}, at); err == nil {
				t.Fatal("unsupported precision accepted after valid save")
			}
			unchanged, err := store.GetTracker(ctx, driver.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, unchanged) {
				t.Fatal("rejected precision changed existing valid progress")
			}

		})
	}
}

func TestProfileHoursAcceptEverySupportedTenth(t *testing.T) {
	for tenths := 0; tenths <= model.MaxRecordedHours*10; tenths++ {
		hours := float64(tenths) / 10
		if err := validateProfileHours(model.Profile{TotalHours: hours, NightHours: hours}); err != nil {
			t.Fatalf("valid hours %v rejected: %v", hours, err)
		}
	}
}
