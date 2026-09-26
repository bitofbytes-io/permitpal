package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/drywaters/permitpal/internal/config"
	"github.com/drywaters/permitpal/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	EnsureDriver(ctx context.Context, username string, now time.Time) (model.Driver, error)
	DriverByUsername(ctx context.Context, username string) (model.Driver, error)
	GetDashboard(ctx context.Context, driver model.Driver, now time.Time) (model.Dashboard, error)
	UpdateProfile(ctx context.Context, driverID int64, profile model.Profile) (model.Profile, error)
	UpdateRequirement(ctx context.Context, driverID int64, req model.Requirement) (model.Requirement, error)
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
		store := NewPostgresStore(pool)
		return store, pool.Close, nil
	default:
		return nil, nil, fmt.Errorf("unsupported DATA_STORE %q", cfg.DataStore)
	}
}
