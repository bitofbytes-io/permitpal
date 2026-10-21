package repository

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/drywaters/permitpal/internal/config"
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

func NewStore(ctx context.Context, cfg *config.Config) (Store, CloseFunc, error) {
	switch cfg.DataStore {
	case config.DataStoreMemory:
		return NewMemoryStore(time.Now()), func() {}, nil
	case config.DataStorePostgres:
		pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, nil, fmt.Errorf("connect postgres: %w", err)
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			return nil, nil, fmt.Errorf("ping postgres: %w", err)
		}
		if err := CheckSchemaVersion(ctx, pool); err != nil {
			pool.Close()
			return nil, nil, err
		}
		store := NewPostgresStore(pool)
		return store, pool.Close, nil
	default:
		return nil, nil, fmt.Errorf("unsupported DATA_STORE %q", cfg.DataStore)
	}
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
