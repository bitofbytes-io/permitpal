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
