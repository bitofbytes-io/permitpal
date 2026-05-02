package repository

import (
	"context"
	"sync"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

type MemoryStore struct {
	mu           sync.RWMutex
	profile      model.Profile
	requirements []model.Requirement
}

func NewMemoryStore(now time.Time) *MemoryStore {
	return &MemoryStore{
		profile:      model.DefaultProfile(now),
		requirements: model.DefaultRequirements(now),
	}
}

func (s *MemoryStore) GetDashboard(_ context.Context, now time.Time) (model.Dashboard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	profile := s.profile
	if s.profile.PermitIssueDate != nil {
		issueDate := *s.profile.PermitIssueDate
		profile.PermitIssueDate = &issueDate
	}

	requirements := make([]model.Requirement, len(s.requirements))
	for idx := range s.requirements {
		requirements[idx] = s.requirements[idx]
		if s.requirements[idx].MasteredDate != nil {
			masteredDate := *s.requirements[idx].MasteredDate
			requirements[idx].MasteredDate = &masteredDate
		}
	}
	return model.NewDashboard(profile, requirements, now), nil
}

func (s *MemoryStore) UpdateProfile(_ context.Context, profile model.Profile) (model.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	profile.UpdatedAt = time.Now()
	s.profile = profile
	return s.profile, nil
}

func (s *MemoryStore) UpdateRequirement(_ context.Context, req model.Requirement) (model.Requirement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for idx, existing := range s.requirements {
		if existing.Key == req.Key {
			req.Title = existing.Title
			req.Description = existing.Description
			req.SortOrder = existing.SortOrder
			req.UpdatedAt = time.Now()
			s.requirements[idx] = req
			return req, nil
		}
	}
	return model.Requirement{}, ErrNotFound
}
