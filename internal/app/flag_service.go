package app

import (
	"context"
	"strings"
	"time"

	"github.com/hrtsegv/flagship/internal/domain"
	"github.com/hrtsegv/flagship/internal/idgen"
)

// FlagService implements flag use cases.
type FlagService struct {
	flags        FlagRepository
	environments EnvironmentRepository
}

// NewFlagService returns a FlagService backed by flags, using environments
// to verify a parent environment exists.
func NewFlagService(flags FlagRepository, environments EnvironmentRepository) *FlagService {
	return &FlagService{flags: flags, environments: environments}
}

// CreateFlagInput holds the fields a caller supplies to create a flag.
type CreateFlagInput struct {
	Key         string
	Name        string
	Description string
	Enabled     bool
}

// Create validates input, verifies environmentID exists, and creates a new
// flag with a server-generated ID starting at version 1.
func (s *FlagService) Create(ctx context.Context, environmentID string, input CreateFlagInput) (domain.Flag, error) {
	if _, err := s.environments.GetByID(ctx, environmentID); err != nil {
		return domain.Flag{}, err
	}

	key := strings.TrimSpace(input.Key)
	name := strings.TrimSpace(input.Name)

	if err := domain.ValidateKey(key); err != nil {
		return domain.Flag{}, err
	}
	if err := domain.ValidateName(name); err != nil {
		return domain.Flag{}, err
	}

	now := time.Now().UTC()
	flag := domain.Flag{
		ID:            idgen.New(),
		EnvironmentID: environmentID,
		Key:           key,
		Name:          name,
		Description:   strings.TrimSpace(input.Description),
		Enabled:       input.Enabled,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	return s.flags.Create(ctx, flag)
}

// ListByEnvironment verifies environmentID exists and returns its flags.
func (s *FlagService) ListByEnvironment(ctx context.Context, environmentID string) ([]domain.Flag, error) {
	if _, err := s.environments.GetByID(ctx, environmentID); err != nil {
		return nil, err
	}
	return s.flags.ListByEnvironment(ctx, environmentID)
}

// Get returns the flag with the given ID.
func (s *FlagService) Get(ctx context.Context, flagID string) (domain.Flag, error) {
	return s.flags.GetByID(ctx, flagID)
}

// Update validates patch and applies it to the flag with the given ID,
// incrementing its version.
func (s *FlagService) Update(ctx context.Context, flagID string, patch domain.FlagPatch) (domain.Flag, error) {
	if patch.Name != nil {
		trimmed := strings.TrimSpace(*patch.Name)
		if err := domain.ValidateName(trimmed); err != nil {
			return domain.Flag{}, err
		}
		patch.Name = &trimmed
	}
	if patch.Description != nil {
		trimmed := strings.TrimSpace(*patch.Description)
		patch.Description = &trimmed
	}

	return s.flags.Update(ctx, flagID, patch)
}

// Delete removes the flag with the given ID.
func (s *FlagService) Delete(ctx context.Context, flagID string) error {
	return s.flags.Delete(ctx, flagID)
}

// EventsSince verifies environmentID exists and returns the events a
// client resuming from sinceVersion missed, along with whether replaying
// them is sufficient to bring that client up to date. When it isn't, the
// caller must fall back to Snapshot.
func (s *FlagService) EventsSince(ctx context.Context, environmentID string, sinceVersion int) ([]domain.ConfigEvent, bool, error) {
	if _, err := s.environments.GetByID(ctx, environmentID); err != nil {
		return nil, false, err
	}
	return s.flags.EventsSince(ctx, environmentID, sinceVersion)
}

// Snapshot verifies environmentID exists and returns its current
// configuration: the environment itself (so callers can read its Key),
// the configuration version, and the flags as of that version.
func (s *FlagService) Snapshot(ctx context.Context, environmentID string) (domain.Environment, int, []domain.Flag, error) {
	env, err := s.environments.GetByID(ctx, environmentID)
	if err != nil {
		return domain.Environment{}, 0, nil, err
	}

	version, flags, err := s.flags.Snapshot(ctx, environmentID)
	if err != nil {
		return domain.Environment{}, 0, nil, err
	}
	return env, version, flags, nil
}
