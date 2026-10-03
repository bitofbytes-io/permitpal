package repository

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSchemaVersionMatchesMigrations(t *testing.T) {
	paths, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	var latest int64
	for _, path := range paths {
		prefix, _, _ := strings.Cut(filepath.Base(path), "_")
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			t.Fatalf("migration %s has no numeric version: %v", path, err)
		}
		latest = max(latest, version)
	}
	if latest != SchemaVersion {
		t.Fatalf("SchemaVersion = %d, latest migration = %d; update SchemaVersion", SchemaVersion, latest)
	}
}

func TestCheckSchemaVersion(t *testing.T) {
	pool := testSchemaPool(t)
	ctx := context.Background()
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	expectErr := func(want bool) {
		t.Helper()
		err := CheckSchemaVersion(ctx, pool)
		if (err != nil) != want {
			t.Fatalf("CheckSchemaVersion error = %v, want error %v", err, want)
		}
		if err != nil && !strings.Contains(err.Error(), "older than required version") {
			t.Fatalf("unclear error: %v", err)
		}
	}

	expectErr(true) // no goose table
	exec(`create table goose_db_version (id serial primary key, version_id bigint not null, is_applied boolean not null, tstamp timestamp default now())`)
	exec(`insert into goose_db_version (version_id, is_applied) values (0, true)`)
	expectErr(true)
	for version := int64(1); version < SchemaVersion; version++ {
		exec(fmt.Sprintf(`insert into goose_db_version (version_id, is_applied) values (%d, true)`, version))
	}
	expectErr(true)
	exec(fmt.Sprintf(`insert into goose_db_version (version_id, is_applied) values (%d, true)`, SchemaVersion))
	expectErr(false)
	exec(fmt.Sprintf(`insert into goose_db_version (version_id, is_applied) values (%d, false)`, SchemaVersion))
	expectErr(true) // rolled back
	exec(fmt.Sprintf(`insert into goose_db_version (version_id, is_applied) values (%d, true)`, SchemaVersion+1))
	expectErr(false) // a newer schema is left to the operator
}

func testSchemaPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("PERMITPAL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PERMITPAL_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := pgx.Identifier{fmt.Sprintf("permitpal_test_%x", random)}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = strings.Trim(schema, `"`)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// migrate applies the Up section of every migration from first through last.
func migrate(t *testing.T, pool *pgxpool.Pool, first, last int64) {
	t.Helper()
	paths, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		prefix, _, _ := strings.Cut(filepath.Base(path), "_")
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if version < first || version > last {
			continue
		}
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(migration), "-- +goose Down")
		if _, err := pool.Exec(context.Background(), up); err != nil {
			t.Fatalf("apply %s: %v", path, err)
		}
	}
}

func TestNightHoursConstraintMigration(t *testing.T) {
	ctx := context.Background()
	constraintValidated := func(t *testing.T, pool *pgxpool.Pool) bool {
		t.Helper()
		var validated bool
		if err := pool.QueryRow(ctx, `select convalidated from pg_constraint where conrelid = 'app_profile'::regclass and conname = 'app_profile_night_hours_within_total'`).Scan(&validated); err != nil {
			t.Fatal(err)
		}
		return validated
	}
	saveHours := func(pool *pgxpool.Pool, driverID int64, total, night float64) error {
		_, err := NewPostgresStore(pool).UpdateProfile(ctx, driverID, model.Profile{TotalHours: total, NightHours: night})
		return err
	}

	t.Run("existing data within the rule", func(t *testing.T) {
		pool := testSchemaPool(t)
		migrate(t, pool, 1, 4)
		migrate(t, pool, 5, 5)
		if !constraintValidated(t, pool) {
			t.Fatal("constraint was not validated although every row satisfies it")
		}
		if err := saveHours(pool, 1, 3, 4); err == nil || !strings.Contains(err.Error(), "app_profile_night_hours_within_total") {
			t.Fatalf("night hours above total were saved: %v", err)
		}
	})

	t.Run("existing row breaks the rule", func(t *testing.T) {
		pool := testSchemaPool(t)
		migrate(t, pool, 1, 4)
		var aiden int64
		if err := pool.QueryRow(ctx, `insert into drivers (username, display_name) values ('aiden', 'Aiden') returning id`).Scan(&aiden); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `insert into app_profile (driver_id, total_hours, night_hours) values ($1, 3, 5)`, aiden); err != nil {
			t.Fatal(err)
		}
		migrate(t, pool, 5, 5)
		if constraintValidated(t, pool) {
			t.Fatal("constraint marked valid over a row that breaks it")
		}
		var total, night float64
		if err := pool.QueryRow(ctx, `select total_hours, night_hours from app_profile where driver_id = $1`, aiden).Scan(&total, &night); err != nil || total != 3 || night != 5 {
			t.Fatalf("existing row changed to total=%v night=%v (%v)", total, night, err)
		}
		if err := saveHours(pool, aiden, 3, 5); err == nil {
			t.Fatal("re-saving night hours above total was accepted")
		}
		if err := saveHours(pool, aiden, 6, 5); err != nil {
			t.Fatalf("a corrected save failed: %v", err)
		}
	})
}

func TestUnusedSeedIsRemovedFromNewInstalls(t *testing.T) {
	ctx := context.Background()
	// install applies migrations 001-005 and records them as goose does:
	// 000-002 applied created ago and 003-005 applied migrated ago.
	install := func(t *testing.T, created, migrated time.Duration) *pgxpool.Pool {
		t.Helper()
		pool := testSchemaPool(t)
		migrate(t, pool, 1, 5)
		if _, err := pool.Exec(ctx, `create table goose_db_version (id serial primary key, version_id bigint not null, is_applied boolean not null, tstamp timestamp default now())`); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `insert into goose_db_version (version_id, is_applied, tstamp) select v, true, localtimestamp - case when v < 3 then $1::interval else $2::interval end from generate_series(0, 5) v`,
			fmt.Sprintf("%d seconds", int(created.Seconds())), fmt.Sprintf("%d seconds", int(migrated.Seconds()))); err != nil {
			t.Fatal(err)
		}
		return pool
	}
	calebTracker := func(t *testing.T, pool *pgxpool.Pool) (int, error) {
		t.Helper()
		store := NewPostgresStore(pool)
		driver, err := store.DriverByUsername(ctx, "caleb")
		if err != nil {
			return 0, err
		}
		tracker, err := store.GetTracker(ctx, driver.ID)
		return len(tracker.Requirements), err
	}

	t.Run("new install", func(t *testing.T) {
		pool := install(t, 0, 0)
		migrate(t, pool, 6, 6)
		if _, err := calebTracker(t, pool); err != ErrNotFound {
			t.Fatalf("seeded caleb driver still present: %v", err)
		}
		var rows int
		if err := pool.QueryRow(ctx, `select (select count(*) from app_profile) + (select count(*) from requirement_items)`).Scan(&rows); err != nil || rows != 0 {
			t.Fatalf("%d seeded rows left (%v)", rows, err)
		}
		if _, err := NewPostgresStore(pool).EnsureDriver(ctx, "caleb", time.Now()); err != nil {
			t.Fatal(err)
		}
		if skills, err := calebTracker(t, pool); err != nil || skills != 17 {
			t.Fatalf("caleb's first login got %d skills (%v), want a fresh 17", skills, err)
		}
	})

	t.Run("original install", func(t *testing.T) {
		pool := install(t, 150*24*time.Hour, 7*24*time.Hour)
		migrate(t, pool, 6, 6)
		if skills, err := calebTracker(t, pool); err != nil || skills != 13 {
			t.Fatalf("original tracker has %d skills (%v), want 13", skills, err)
		}
	})

	t.Run("install created by an earlier goose run", func(t *testing.T) {
		// Someone may have signed in to the seeded tracker since, without saving.
		pool := install(t, 2*time.Hour, 2*time.Hour)
		migrate(t, pool, 6, 6)
		if skills, err := calebTracker(t, pool); err != nil || skills != 13 {
			t.Fatalf("tracker has %d skills (%v), want 13", skills, err)
		}
	})

	t.Run("new install whose seed was used", func(t *testing.T) {
		pool := install(t, 0, 0)
		store := NewPostgresStore(pool)
		caleb, err := store.DriverByUsername(ctx, "caleb")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.UpdateRequirement(ctx, caleb.ID, model.Requirement{Key: "backing", Rating: model.RatingGood}); err != nil {
			t.Fatal(err)
		}
		migrate(t, pool, 6, 6)
		if skills, err := calebTracker(t, pool); err != nil || skills != 13 {
			t.Fatalf("used tracker has %d skills (%v), want 13", skills, err)
		}
	})

	t.Run("without goose records", func(t *testing.T) {
		pool := testSchemaPool(t)
		migrate(t, pool, 1, 6)
		if skills, err := calebTracker(t, pool); err != nil || skills != 13 {
			t.Fatalf("tracker has %d skills (%v), want 13", skills, err)
		}
	})
}
