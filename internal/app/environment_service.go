package app

import (
	"context"
	"strings"
	"time"

	"github.com/hrtsegv/flagship/internal/domain"
	"github.com/hrtsegv/flagship/internal/idgen"
)

// EnvironmentService implements environment use cases.
type EnvironmentService struct {
	environments EnvironmentRepository
	projects     ProjectRepository
}

// NewEnvironmentService returns an EnvironmentService backed by
// environments, using projects to verify a parent project exists.
func NewEnvironmentService(environments EnvironmentRepository, projects ProjectRepository) *EnvironmentService {
	return &EnvironmentService{environments: environments, projects: projects}
}

// CreateEnvironmentInput holds the fields a caller supplies to create an
// environment.
type CreateEnvironmentInput struct {
	Key  string
	Name string
}

// Create validates input, verifies projectID exists, and creates a new
// environment with a server-generated ID.
func (s *EnvironmentService) Create(ctx context.Context, projectID string, input CreateEnvironmentInput) (domain.Environment, error) {
	if _, err := s.projects.GetByID(ctx, projectID); err != nil {
		return domain.Environment{}, err
	}

	key := strings.TrimSpace(input.Key)
	name := strings.TrimSpace(input.Name)

	if err := domain.ValidateKey(key); err != nil {
		return domain.Environment{}, err
	}
	if err := domain.ValidateName(name); err != nil {
		return domain.Environment{}, err
	}

	now := time.Now().UTC()
	env := domain.Environment{
		ID:        idgen.New(),
		ProjectID: projectID,
		Key:       key,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return s.environments.Create(ctx, env)
}

// ListByProject verifies projectID exists and returns its environments.
func (s *EnvironmentService) ListByProject(ctx context.Context, projectID string) ([]domain.Environment, error) {
	if _, err := s.projects.GetByID(ctx, projectID); err != nil {
		return nil, err
	}
	return s.environments.ListByProject(ctx, projectID)
}
