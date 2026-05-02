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

func (s *PostgresStore) GetDashboard(ctx context.Context, now time.Time) (model.Dashboard, error) {
	profile, err := s.getProfile(ctx)
	if err != nil {
		return model.Dashboard{}, err
	}
	requirements, err := s.getRequirements(ctx)
	if err != nil {
		return model.Dashboard{}, err
	}
	return model.NewDashboard(profile, requirements, now), nil
}

func (s *PostgresStore) UpdateProfile(ctx context.Context, profile model.Profile) (model.Profile, error) {
	const query = `
		insert into app_profile (id, permit_issue_date, total_hours, night_hours, updated_at)
		values (1, $1, $2, $3, now())
		on conflict (id) do update set
			permit_issue_date = excluded.permit_issue_date,
			total_hours = excluded.total_hours,
			night_hours = excluded.night_hours,
			updated_at = now()
		returning permit_issue_date, total_hours, night_hours, updated_at`
	err := s.db.QueryRow(ctx, query, profile.PermitIssueDate, profile.TotalHours, profile.NightHours).
		Scan(&profile.PermitIssueDate, &profile.TotalHours, &profile.NightHours, &profile.UpdatedAt)
	return profile, err
}

func (s *PostgresStore) UpdateRequirement(ctx context.Context, req model.Requirement) (model.Requirement, error) {
	const query = `
		update requirement_items
		set status = $2, mastered_date = $3, notes = $4, updated_at = now()
		where key = $1
		returning key, title, description, status, mastered_date, notes, sort_order, updated_at`
	err := s.db.QueryRow(ctx, query, req.Key, req.Status, req.MasteredDate, req.Notes).
		Scan(&req.Key, &req.Title, &req.Description, &req.Status, &req.MasteredDate, &req.Notes, &req.SortOrder, &req.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Requirement{}, ErrNotFound
	}
	return req, err
}

func (s *PostgresStore) getProfile(ctx context.Context) (model.Profile, error) {
	const query = `select permit_issue_date, total_hours, night_hours, updated_at from app_profile where id = 1`
	var profile model.Profile
	err := s.db.QueryRow(ctx, query).
		Scan(&profile.PermitIssueDate, &profile.TotalHours, &profile.NightHours, &profile.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.UpdateProfile(ctx, model.DefaultProfile(time.Now()))
	}
	return profile, err
}

func (s *PostgresStore) getRequirements(ctx context.Context) ([]model.Requirement, error) {
	rows, err := s.db.Query(ctx, `
		select key, title, description, status, mastered_date, notes, sort_order, updated_at
		from requirement_items
		order by sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requirements []model.Requirement
	for rows.Next() {
		var req model.Requirement
		if err := rows.Scan(&req.Key, &req.Title, &req.Description, &req.Status, &req.MasteredDate, &req.Notes, &req.SortOrder, &req.UpdatedAt); err != nil {
			return nil, err
		}
		requirements = append(requirements, req)
	}
	return requirements, rows.Err()
}
