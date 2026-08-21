package app

import (
	"context"
	"errors"
	"testing"

	"github.com/hrtsegv/flagship/internal/domain"
	"github.com/hrtsegv/flagship/internal/memory"
)

func newProjectForEnvTest(t *testing.T, ctx context.Context, projects *memory.ProjectRepository) domain.Project {
	t.Helper()
	svc := NewProjectService(projects)
	p, err := svc.Create(ctx, CreateProjectInput{Key: "ecommerce", Name: "Ecommerce"})
	if err != nil {
		t.Fatalf("failed to create prerequisite project: %v", err)
	}
	return p
}

func TestEnvironmentService_Create(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	project := newProjectForEnvTest(t, ctx, projects)

	svc := NewEnvironmentService(memory.NewEnvironmentRepository(), projects)
	env, err := svc.Create(ctx, project.ID, CreateEnvironmentInput{Key: "production", Name: "Production"})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if env.ProjectID != project.ID {
		t.Errorf("Create() ProjectID = %q, want %q", env.ProjectID, project.ID)
	}
	if env.ID == "" {
		t.Error("Create() did not assign an ID")
	}
}

func TestEnvironmentService_Create_ProjectNotFound(t *testing.T) {
	ctx := context.Background()
	svc := NewEnvironmentService(memory.NewEnvironmentRepository(), memory.NewProjectRepository())

	_, err := svc.Create(ctx, "missing-project", CreateEnvironmentInput{Key: "production", Name: "Production"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Create() error = %v, want ErrNotFound", err)
	}
}

func TestEnvironmentService_Create_DuplicateKey(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	project := newProjectForEnvTest(t, ctx, projects)

	svc := NewEnvironmentService(memory.NewEnvironmentRepository(), projects)
	if _, err := svc.Create(ctx, project.ID, CreateEnvironmentInput{Key: "production", Name: "Production"}); err != nil {
		t.Fatalf("first Create() unexpected error: %v", err)
	}
	_, err := svc.Create(ctx, project.ID, CreateEnvironmentInput{Key: "production", Name: "Production Again"})
	if !errors.Is(err, domain.ErrDuplicateKey) {
		t.Fatalf("second Create() error = %v, want ErrDuplicateKey", err)
	}
}

func TestEnvironmentService_ListByProject_ProjectNotFound(t *testing.T) {
	ctx := context.Background()
	svc := NewEnvironmentService(memory.NewEnvironmentRepository(), memory.NewProjectRepository())

	_, err := svc.ListByProject(ctx, "missing-project")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ListByProject() error = %v, want ErrNotFound", err)
	}
}

func TestEnvironmentService_ListByProject(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	project := newProjectForEnvTest(t, ctx, projects)

	svc := NewEnvironmentService(memory.NewEnvironmentRepository(), projects)
	if _, err := svc.Create(ctx, project.ID, CreateEnvironmentInput{Key: "production", Name: "Production"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, project.ID, CreateEnvironmentInput{Key: "staging", Name: "Staging"}); err != nil {
		t.Fatal(err)
	}

	list, err := svc.ListByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListByProject() unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListByProject() returned %d environments, want 2", len(list))
	}
}
