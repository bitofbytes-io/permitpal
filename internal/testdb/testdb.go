// Package testdb provides isolated Postgres schemas for tests.
package testdb

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewEmpty returns a pool whose search_path is a new empty schema in the
// disposable database named by PERMITPAL_TEST_DATABASE_URL. The test is
// skipped when the variable is unset, and the schema is dropped on cleanup.
func NewEmpty(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := NewEmptyWithURL(t)
	return pool
}

// NewEmptyWithURL is NewEmpty that also returns a database URL selecting the
// same schema, for code (or a child process) that opens its own connections.
func NewEmptyWithURL(t *testing.T) (*pgxpool.Pool, string) {
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
	schemaURL, err := SetConnParam(databaseURL, "search_path", cfg.ConnConfig.RuntimeParams["search_path"])
	if err != nil {
		t.Fatal(err)
	}
	return pool, schemaURL
}
