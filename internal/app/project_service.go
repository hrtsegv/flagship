package app

import (
	"context"
	"strings"
	"time"

	"github.com/hrtsegv/flagship/internal/domain"
	"github.com/hrtsegv/flagship/internal/idgen"
)

// ProjectService implements project use cases.
type ProjectService struct {
	projects ProjectRepository
}

// NewProjectService returns a ProjectService backed by projects.
func NewProjectService(projects ProjectRepository) *ProjectService {
	return &ProjectService{projects: projects}
}

// CreateProjectInput holds the fields a caller supplies to create a
// project.
type CreateProjectInput struct {
	Key         string
	Name        string
	Description string
}

// Create validates input and creates a new project with a server-generated
// ID.
func (s *ProjectService) Create(ctx context.Context, input CreateProjectInput) (domain.Project, error) {
	key := strings.TrimSpace(input.Key)
	name := strings.TrimSpace(input.Name)

	if err := domain.ValidateKey(key); err != nil {
		return domain.Project{}, err
	}
	if err := domain.ValidateName(name); err != nil {
		return domain.Project{}, err
	}

	now := time.Now().UTC()
	project := domain.Project{
		ID:          idgen.New(),
		Key:         key,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return s.projects.Create(ctx, project)
}

// List returns all projects.
func (s *ProjectService) List(ctx context.Context) ([]domain.Project, error) {
	return s.projects.List(ctx)
}
