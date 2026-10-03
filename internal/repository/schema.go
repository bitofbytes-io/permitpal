package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SchemaVersion is the highest goose migration in migrations/ that this
// binary requires. TestSchemaVersionMatchesMigrations keeps it in sync.
const SchemaVersion int64 = 5

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CheckSchemaVersion refuses a database whose applied goose version is older
// than SchemaVersion, so an auto-deployed binary does not run on an
// unmigrated schema.
func CheckSchemaVersion(ctx context.Context, db rowQuerier) error {
	var exists bool
	if err := db.QueryRow(ctx, `select to_regclass('goose_db_version') is not null`).Scan(&exists); err != nil {
		return fmt.Errorf("check schema version: %w", err)
	}
	var version int64
	if exists {
		// goose records rollbacks as new rows, so only a version's latest row counts.
		const query = `
			select coalesce(max(version_id), 0) from (
				select distinct on (version_id) version_id, is_applied
				from goose_db_version
				order by version_id, id desc
			) latest where is_applied`
		if err := db.QueryRow(ctx, query).Scan(&version); err != nil {
			return fmt.Errorf("check schema version: %w", err)
		}
	}
	if version < SchemaVersion {
		return fmt.Errorf("database schema version %d is older than required version %d; apply migrations (make migrate) before starting this release", version, SchemaVersion)
	}
	return nil
}
