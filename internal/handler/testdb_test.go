package handler

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// forEachBackend runs a test body once against the memory store and once
// against Postgres (skipped when no test database URL is set).
func forEachBackend(t *testing.T, run func(t *testing.T, backend string)) {
	t.Helper()
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) { run(t, backend) })
	}
}

// Each database test uses its own schema and the production migrations. The
// explicitly supplied test URL must point to a disposable PostgreSQL database.
func validationStore(t *testing.T, backend string, now time.Time) repository.Store {
	t.Helper()
	if backend == "memory" {
		return repository.NewMemoryStore(now)
	}
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
	schema := fmt.Sprintf("permitpal_test_%x", random)
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migrations, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	for _, path := range migrations {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		up, down, found := strings.Cut(string(migration), "-- +goose Down")
		if !found {
			t.Fatalf("migration %s has no down boundary", path)
		}
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", path, err)
		}
		if filepath.Base(path) != "003_multi_driver_ratings.sql" {
			continue
		}
		// Exercise the exact rollback SQL in the same non-public schema, then
		// restore it, before later migrations build on 003's tables.
		beforeRows := migrationSnapshot(t, pool)
		for _, sql := range []string{down, up} {
			if _, err := pool.Exec(ctx, sql); err != nil {
				t.Fatalf("migration 003 down/up: %v", err)
			}
		}
		if afterRows := migrationSnapshot(t, pool); !reflect.DeepEqual(beforeRows, afterRows) {
			t.Fatal("migration 003 down/up changed Caleb history")
		}
	}
	return repository.NewPostgresStore(pool)
}

func migrationSnapshot(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(context.Background(), `select json_build_object('profile',(select row_to_json(p) from (select permit_issue_date,total_hours,night_hours,updated_at from app_profile) p),'requirements',(select json_agg(r order by sort_order) from (select key,title,description,status,rated_on,notes,sort_order,updated_at from requirement_items) r))::text`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
