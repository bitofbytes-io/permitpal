package repository

import (
	"context"
	"errors"
	"time"

	"github.com/drywaters/permitpal/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	db *pgxpool.Pool
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) GetDashboard(ctx context.Context, driver model.Driver, now time.Time) (model.Dashboard, error) {
	profile, err := s.getProfile(ctx, driver.ID, now)
	if err != nil {
		return model.Dashboard{}, err
	}
	requirements, err := s.getRequirements(ctx, driver.ID)
	if err != nil {
		return model.Dashboard{}, err
	}
	return model.NewDashboard(driver, profile, requirements, now), nil
}

func (s *PostgresStore) UpdateProfile(ctx context.Context, driverID int64, profile model.Profile) (model.Profile, error) {
	const query = `
		insert into app_profile (driver_id, permit_issue_date, total_hours, night_hours, updated_at)
		values ($4, $1, $2, $3, now())
		on conflict (driver_id) do update set
			permit_issue_date = excluded.permit_issue_date,
			total_hours = excluded.total_hours,
			night_hours = excluded.night_hours,
			updated_at = now()
		returning permit_issue_date, total_hours, night_hours, updated_at`
	err := s.db.QueryRow(ctx, query, profile.PermitIssueDate, profile.TotalHours, profile.NightHours, driverID).
		Scan(&profile.PermitIssueDate, &profile.TotalHours, &profile.NightHours, &profile.UpdatedAt)
	return profile, err
}

func (s *PostgresStore) UpdateRequirement(ctx context.Context, driverID int64, req model.Requirement) (model.Requirement, error) {
	const query = `
		update requirement_items
		set status = $2, rated_on = $3, notes = $4, updated_at = now()
		where key = $1 and driver_id = $5
		returning key, title, description, status, rated_on, notes, sort_order, updated_at`
	err := s.db.QueryRow(ctx, query, req.Key, req.Rating, req.RatedOn, req.Notes, driverID).
		Scan(&req.Key, &req.Title, &req.Description, &req.Rating, &req.RatedOn, &req.Notes, &req.SortOrder, &req.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Requirement{}, ErrNotFound
	}
	return req, err
}

func (s *PostgresStore) getProfile(ctx context.Context, driverID int64, now time.Time) (model.Profile, error) {
	const query = `select permit_issue_date, total_hours, night_hours, updated_at from app_profile where driver_id = $1`
	var profile model.Profile
	err := s.db.QueryRow(ctx, query, driverID).
		Scan(&profile.PermitIssueDate, &profile.TotalHours, &profile.NightHours, &profile.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent login or update may create the row after our initial read.
		// Do not overwrite its progress while repairing a missing profile.
		if _, err = s.db.Exec(ctx, `insert into app_profile (driver_id, updated_at) values ($1,$2) on conflict (driver_id) do nothing`, driverID, now); err != nil {
			return model.Profile{}, err
		}
		err = s.db.QueryRow(ctx, query, driverID).Scan(&profile.PermitIssueDate, &profile.TotalHours, &profile.NightHours, &profile.UpdatedAt)
	}
	return profile, err
}

func (s *PostgresStore) getRequirements(ctx context.Context, driverID int64) ([]model.Requirement, error) {
	rows, err := s.db.Query(ctx, `
		select key, title, description, status, rated_on, notes, sort_order, updated_at
		from requirement_items where driver_id = $1
		order by sort_order`, driverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requirements []model.Requirement
	for rows.Next() {
		var req model.Requirement
		if err := rows.Scan(&req.Key, &req.Title, &req.Description, &req.Rating, &req.RatedOn, &req.Notes, &req.SortOrder, &req.UpdatedAt); err != nil {
			return nil, err
		}
		requirements = append(requirements, req)
	}
	return requirements, rows.Err()
}

func (s *PostgresStore) DriverByUsername(ctx context.Context, username string) (model.Driver, error) {
	var driver model.Driver
	err := s.db.QueryRow(ctx, `select id,username,display_name from drivers where username=$1`, username).Scan(&driver.ID, &driver.Username, &driver.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Driver{}, ErrNotFound
	}
	return driver, err
}
func (s *PostgresStore) EnsureDriver(ctx context.Context, username string, now time.Time) (model.Driver, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return model.Driver{}, err
	}
	defer tx.Rollback(ctx)
	driver := model.NewDriver(username)
	err = tx.QueryRow(ctx, `insert into drivers (username,display_name,created_at) values ($1,$2,$3) on conflict (username) do nothing returning id`, username, driver.DisplayName, now).Scan(&driver.ID)
	created := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return model.Driver{}, err
	}
	err = tx.QueryRow(ctx, `select id,username,display_name from drivers where username=$1`, username).Scan(&driver.ID, &driver.Username, &driver.DisplayName)
	if err != nil {
		return model.Driver{}, err
	}
	if _, err = tx.Exec(ctx, `insert into app_profile (driver_id,updated_at) values ($1,$2) on conflict (driver_id) do nothing`, driver.ID, now); err != nil {
		return model.Driver{}, err
	}
	// Existing drivers retain their exact checklist, including Caleb's 13 legacy items.
	if created {
		for _, req := range model.DefaultRequirements(now) {
			if _, err = tx.Exec(ctx, `insert into requirement_items (driver_id,key,title,description,status,sort_order,updated_at) values ($1,$2,$3,$4,$5,$6,$7) on conflict do nothing`, driver.ID, req.Key, req.Title, req.Description, req.Rating, req.SortOrder, now); err != nil {
				return model.Driver{}, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return model.Driver{}, err
	}
	return driver, nil
}
