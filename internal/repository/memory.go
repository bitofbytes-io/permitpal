package repository

import (
	"context"
	"sync"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

type driverState struct {
	driver       model.Driver
	profile      model.Profile
	requirements []model.Requirement
}
type MemoryStore struct {
	mu      sync.RWMutex
	drivers map[string]*driverState
}

func NewMemoryStore(_ time.Time) *MemoryStore {
	return &MemoryStore{drivers: make(map[string]*driverState)}
}
func (s *MemoryStore) EnsureDriver(_ context.Context, username string, now time.Time) (model.Driver, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state, ok := s.drivers[username]; ok {
		return state.driver, nil
	}
	driver := model.NewDriver(username)
	driver.ID = int64(len(s.drivers) + 1)
	s.drivers[username] = &driverState{driver, model.NewDriverProfile(now), model.DefaultRequirements(now)}
	return driver, nil
}
func (s *MemoryStore) DriverByUsername(_ context.Context, username string) (model.Driver, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if state, ok := s.drivers[username]; ok {
		return state.driver, nil
	}
	return model.Driver{}, ErrNotFound
}
func (s *MemoryStore) stateByID(id int64) *driverState {
	for _, state := range s.drivers {
		if state.driver.ID == id {
			return state
		}
	}
	return nil
}
func copyProfile(profile model.Profile) model.Profile {
	if profile.PermitIssueDate != nil {
		date := *profile.PermitIssueDate
		profile.PermitIssueDate = &date
	}
	return profile
}
func copyRequirement(req model.Requirement) model.Requirement {
	if req.RatedOn != nil {
		date := *req.RatedOn
		req.RatedOn = &date
	}
	return req
}
func (s *MemoryStore) GetTracker(_ context.Context, driverID int64) (model.Tracker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.stateByID(driverID)
	if state == nil {
		return model.Tracker{}, ErrNotFound
	}
	requirements := make([]model.Requirement, len(state.requirements))
	for i, req := range state.requirements {
		requirements[i] = copyRequirement(req)
	}
	return model.Tracker{Profile: copyProfile(state.profile), Requirements: requirements}, nil
}
func (s *MemoryStore) UpdateProfile(_ context.Context, driverID int64, profile model.Profile) (model.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.stateByID(driverID)
	if state == nil {
		return model.Profile{}, ErrNotFound
	}
	profile.UpdatedAt = time.Now()
	state.profile = copyProfile(profile)
	return copyProfile(profile), nil
}
func (s *MemoryStore) UpdateRequirement(_ context.Context, driverID int64, req model.Requirement) (model.Requirement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.stateByID(driverID)
	if state == nil {
		return model.Requirement{}, ErrNotFound
	}
	for i, existing := range state.requirements {
		if existing.Key == req.Key {
			req.Title = existing.Title
			req.Description = existing.Description
			req.SortOrder = existing.SortOrder
			req.UpdatedAt = time.Now()
			state.requirements[i] = copyRequirement(req)
			return copyRequirement(req), nil
		}
	}
	return model.Requirement{}, ErrNotFound
}
