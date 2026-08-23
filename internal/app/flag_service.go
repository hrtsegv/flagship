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

// StreamSubscription is what FlagService.Stream returns: the events
// needed to catch a caller up to the moment of subscription (if
// Replayable), and a live feed of further events from that same moment
// onward. The caller must call Unsubscribe when done (typically via
// defer).
type StreamSubscription struct {
	Environment domain.Environment
	Events      []domain.ConfigEvent
	Replayable  bool
	Live        <-chan domain.ConfigEvent
	Unsubscribe func()
}

// Stream verifies environmentID exists and atomically subscribes to its
// live configuration event feed, alongside the events since sinceVersion
// needed to catch the caller up to the moment of subscription. When
// Replayable is false, the caller has fallen too far behind (or asked for
// a version the server never had) and must recover via Snapshot instead
// of trusting the (empty) event list.
func (s *FlagService) Stream(ctx context.Context, environmentID string, sinceVersion int) (StreamSubscription, error) {
	env, err := s.environments.GetByID(ctx, environmentID)
	if err != nil {
		return StreamSubscription{}, err
	}

	events, replayable, live, unsubscribe := s.flags.SubscribeFrom(ctx, environmentID, sinceVersion)
	return StreamSubscription{
		Environment: env,
		Events:      events,
		Replayable:  replayable,
		Live:        live,
		Unsubscribe: unsubscribe,
	}, nil
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
