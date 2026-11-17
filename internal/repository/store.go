package repository

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/drywaters/permitpal/internal/config"
	dbmigrate "github.com/drywaters/permitpal/internal/migrate"
	"github.com/drywaters/permitpal/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	EnsureDriver(ctx context.Context, username string, now time.Time) (model.Driver, error)
	DriverByUsername(ctx context.Context, username string) (model.Driver, error)
	// EndSessions increments the driver's session generation if it still
	// equals generation, so a stale cookie cannot end newer sessions.
	EndSessions(ctx context.Context, username string, generation int64) error
	GetTracker(ctx context.Context, driverID int64) (model.Tracker, error)
	UpdateProfile(ctx context.Context, driverID int64, profile model.Profile, recordedAt time.Time) (model.Profile, error)
	UpdateRequirement(ctx context.Context, driverID int64, req model.Requirement) (model.Requirement, error)
	// Ping reports whether the backing database is reachable.
	Ping(ctx context.Context) error
}

type CloseFunc func()

// connectTimeout bounds the first database connection. It is the first part of
// the startup budget described in internal/migrate/timeouts.go.
const connectTimeout = 10 * time.Second

// initTimeout is the one deadline shared by connecting, migrating and the
// schema check, matching the 30s healthcheck start period. Releasing the
// migration lock is bounded separately (see internal/migrate/timeouts.go).
const initTimeout = connectTimeout + dbmigrate.StartupTimeout

// postgresInit holds the startup steps so tests can exercise the shared
// deadline with short budgets and blocking steps.
type postgresInit struct {
	timeout time.Duration
	migrate func(ctx context.Context, pool *pgxpool.Pool) error
	check   func(ctx context.Context, pool *pgxpool.Pool) error
}

var defaultPostgresInit = postgresInit{
	timeout: initTimeout,
	migrate: func(ctx context.Context, pool *pgxpool.Pool) error {
		return dbmigrate.Apply(ctx, pool, slog.Default())
	},
	check: func(ctx context.Context, pool *pgxpool.Pool) error {
		return CheckSchemaVersion(ctx, pool)
	},
}

func NewStore(ctx context.Context, cfg *config.Config) (Store, CloseFunc, error) {
	switch cfg.DataStore {
	case config.DataStoreMemory:
		return NewMemoryStore(time.Now()), func() {}, nil
	case config.DataStorePostgres:
		return newPostgresStore(ctx, cfg.DatabaseURL, defaultPostgresInit)
	default:
		return nil, nil, fmt.Errorf("unsupported DATA_STORE %q", cfg.DataStore)
	}
}

func newPostgresStore(ctx context.Context, databaseURL string, steps postgresInit) (Store, CloseFunc, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect postgres: %w", err)
	}
	// One deadline bounds ping, migration and the schema check, so no step can
	// hang startup past the healthcheck start period.
	initCtx, cancel := context.WithTimeout(ctx, steps.timeout)
	defer cancel()
	pingCtx, cancelPing := context.WithTimeout(initCtx, connectTimeout)
	err = pool.Ping(pingCtx)
	cancelPing()
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("ping postgres: %w", err)
	}
	// Apply the embedded migrations before serving. A failure stops startup,
	// so the new task never becomes healthy and Swarm rolls the update back.
	if err := steps.migrate(initCtx, pool); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("apply migrations: %w", err)
	}
	// Safety net: the database must now be at the version this binary needs.
	if err := steps.check(initCtx, pool); err != nil {
		pool.Close()
		return nil, nil, err
	}
	return NewPostgresStore(pool), pool.Close, nil
}

// Both backends reject invalid cumulative values before integer conversion.
func validateProfileHours(profile model.Profile) error {
	for _, hours := range []float64{profile.TotalHours, profile.NightHours} {
		if math.IsNaN(hours) || math.IsInf(hours, 0) || hours < 0 || hours > model.MaxRecordedHours {
			return fmt.Errorf("recorded hours must be finite and between 0 and %d", model.MaxRecordedHours)
		}
		// Compare in hours after rounding to tenths, so ordinary decimal
		// float representations (such as 0.1) remain valid. Never silently
		// round direct Store callers differently from the HTTP form.
		if hours != math.Round(hours*10)/10 {
			return fmt.Errorf("recorded hours must have at most one decimal place")
		}
	}
	if profile.NightHours > profile.TotalHours {
		return fmt.Errorf("night hours cannot exceed total hours")
	}
	return nil
}
