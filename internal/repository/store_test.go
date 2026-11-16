package repository

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/config"
	dbmigrate "github.com/drywaters/permitpal/internal/migrate"
	"github.com/drywaters/permitpal/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func appliedGooseVersions(t *testing.T, pool *pgxpool.Pool) []int64 {
	t.Helper()
	rows, err := pool.Query(context.Background(), `select version_id from goose_db_version where version_id > 0 and is_applied order by id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var versions []int64
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return versions
}

func TestNewStoreMigratesEmptyDatabaseToSchemaVersion(t *testing.T) {
	pool, databaseURL := testdb.NewEmptyWithURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, closeStore, err := NewStore(ctx, &config.Config{DataStore: config.DataStorePostgres, DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("NewStore on an empty database: %v", err)
	}
	defer closeStore()
	if err := CheckSchemaVersion(ctx, pool); err != nil {
		t.Fatalf("CheckSchemaVersion after startup: %v", err)
	}
	versions := appliedGooseVersions(t, pool)
	if len(versions) == 0 || versions[len(versions)-1] != SchemaVersion {
		t.Fatalf("applied versions = %v, want latest %d", versions, SchemaVersion)
	}
	if err := store.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	// A second startup is a no-op and leaves the history alone.
	again, closeAgain, err := NewStore(ctx, &config.Config{DataStore: config.DataStorePostgres, DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("second NewStore: %v", err)
	}
	defer closeAgain()
	if err := again.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if got := appliedGooseVersions(t, pool); len(got) != len(versions) {
		t.Fatalf("second startup changed applied versions: %v -> %v", versions, got)
	}
}

func TestNewStoreMigratesVersion7PreservingData(t *testing.T) {
	pool, databaseURL := testdb.NewEmptyWithURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Records 001-007 in goose_db_version, as the goose CLI would have before 008 shipped.
	recordGooseVersions(t, pool, 1, 7)
	if err := CheckSchemaVersion(ctx, pool); err == nil || !strings.Contains(err.Error(), "older than required version") {
		t.Fatalf("CheckSchemaVersion at version 7 = %v, want older-than-required error", err)
	}
	driver, err := NewPostgresStore(pool).EnsureDriver(ctx, "before-startup-migration", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	store, closeStore, err := NewStore(ctx, &config.Config{DataStore: config.DataStorePostgres, DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("NewStore on a version 7 database: %v", err)
	}
	defer closeStore()
	if err := CheckSchemaVersion(ctx, pool); err != nil {
		t.Fatalf("CheckSchemaVersion after startup: %v", err)
	}
	var roadTest *time.Time
	if err := pool.QueryRow(ctx, `select road_test_date from app_profile where driver_id = $1`, driver.ID).Scan(&roadTest); err != nil {
		t.Fatalf("read road_test_date: %v", err)
	}
	if roadTest != nil {
		t.Fatalf("road_test_date = %v, want none for an existing driver", roadTest)
	}
	got, err := store.DriverByUsername(ctx, "before-startup-migration")
	if err != nil || got.ID != driver.ID {
		t.Fatalf("driver after migration = %+v (%v), want id %d", got, err, driver.ID)
	}
}

// recordGooseVersions applies migrations first..last and records them in
// goose_db_version, which is the state the goose CLI leaves behind.
func recordGooseVersions(t *testing.T, pool *pgxpool.Pool, first, last int64) {
	t.Helper()
	migrate(t, pool, first, last)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `create table goose_db_version (id serial primary key, version_id bigint not null, is_applied boolean not null, tstamp timestamp default now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into goose_db_version (version_id, is_applied) select v, true from generate_series(0, $1::bigint) v`, last); err != nil {
		t.Fatal(err)
	}
}

// Migration 006 deletes the seeded tracker only when goose applied 001 in the
// same run, so a fresh database must start with no drivers when it is created
// through the provider.
func TestApplyOnFreshDatabaseRemovesSeed(t *testing.T) {
	pool := testdb.NewEmpty(t)
	ctx := context.Background()
	if err := dbmigrate.Apply(ctx, pool, discardLogger); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `select (select count(*) from drivers) + (select count(*) from app_profile) + (select count(*) from requirement_items)`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("%d seeded rows left after a fresh install (%v)", rows, err)
	}
	if _, err := NewPostgresStore(pool).EnsureDriver(ctx, "caleb", time.Now()); err != nil {
		t.Fatal(err)
	}
	store := NewPostgresStore(pool)
	driver, err := store.DriverByUsername(ctx, "caleb")
	if err != nil {
		t.Fatal(err)
	}
	requirements, err := store.getRequirements(ctx, driver.ID)
	if err != nil || len(requirements) != 17 {
		t.Fatalf("caleb's first login got %d skills (%v), want a fresh 17", len(requirements), err)
	}
}
