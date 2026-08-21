package app

import (
	"context"
	"errors"
	"testing"

	"github.com/hrtsegv/flagship/internal/domain"
	"github.com/hrtsegv/flagship/internal/memory"
)

func newEnvironmentForFlagTest(t *testing.T, ctx context.Context, projects *memory.ProjectRepository, environments *memory.EnvironmentRepository) domain.Environment {
	t.Helper()
	projectSvc := NewProjectService(projects)
	project, err := projectSvc.Create(ctx, CreateProjectInput{Key: "ecommerce", Name: "Ecommerce"})
	if err != nil {
		t.Fatalf("failed to create prerequisite project: %v", err)
	}

	envSvc := NewEnvironmentService(environments, projects)
	env, err := envSvc.Create(ctx, project.ID, CreateEnvironmentInput{Key: "production", Name: "Production"})
	if err != nil {
		t.Fatalf("failed to create prerequisite environment: %v", err)
	}
	return env
}

func TestFlagService_Create(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	environments := memory.NewEnvironmentRepository()
	env := newEnvironmentForFlagTest(t, ctx, projects, environments)

	svc := NewFlagService(memory.NewFlagRepository(), environments)
	flag, err := svc.Create(ctx, env.ID, CreateFlagInput{Key: "new-checkout", Name: "New Checkout", Enabled: false})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if flag.EnvironmentID != env.ID {
		t.Errorf("Create() EnvironmentID = %q, want %q", flag.EnvironmentID, env.ID)
	}
	if flag.Version != 1 {
		t.Errorf("Create() Version = %d, want 1", flag.Version)
	}
	if flag.ID == "" {
		t.Error("Create() did not assign an ID")
	}
}

func TestFlagService_Create_EnvironmentNotFound(t *testing.T) {
	ctx := context.Background()
	svc := NewFlagService(memory.NewFlagRepository(), memory.NewEnvironmentRepository())

	_, err := svc.Create(ctx, "missing-env", CreateFlagInput{Key: "new-checkout", Name: "New Checkout"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Create() error = %v, want ErrNotFound", err)
	}
}

func TestFlagService_Create_DuplicateKey(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	environments := memory.NewEnvironmentRepository()
	env := newEnvironmentForFlagTest(t, ctx, projects, environments)

	svc := NewFlagService(memory.NewFlagRepository(), environments)
	if _, err := svc.Create(ctx, env.ID, CreateFlagInput{Key: "new-checkout", Name: "New Checkout"}); err != nil {
		t.Fatalf("first Create() unexpected error: %v", err)
	}
	_, err := svc.Create(ctx, env.ID, CreateFlagInput{Key: "new-checkout", Name: "New Checkout Again"})
	if !errors.Is(err, domain.ErrDuplicateKey) {
		t.Fatalf("second Create() error = %v, want ErrDuplicateKey", err)
	}
}

func TestFlagService_Get(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	environments := memory.NewEnvironmentRepository()
	env := newEnvironmentForFlagTest(t, ctx, projects, environments)

	svc := NewFlagService(memory.NewFlagRepository(), environments)
	created, err := svc.Create(ctx, env.ID, CreateFlagInput{Key: "new-checkout", Name: "New Checkout"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.Key != "new-checkout" {
		t.Errorf("Get() Key = %q, want %q", got.Key, "new-checkout")
	}
}

func TestFlagService_Get_NotFound(t *testing.T) {
	svc := NewFlagService(memory.NewFlagRepository(), memory.NewEnvironmentRepository())
	_, err := svc.Get(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestFlagService_Update_IncrementsVersion(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	environments := memory.NewEnvironmentRepository()
	env := newEnvironmentForFlagTest(t, ctx, projects, environments)

	svc := NewFlagService(memory.NewFlagRepository(), environments)
	created, err := svc.Create(ctx, env.ID, CreateFlagInput{Key: "new-checkout", Name: "New Checkout", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if created.Version != 1 {
		t.Fatalf("created Version = %d, want 1", created.Version)
	}

	enabled := true
	updated, err := svc.Update(ctx, created.ID, domain.FlagPatch{Enabled: &enabled})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if !updated.Enabled {
		t.Error("Update() Enabled = false, want true")
	}
	if updated.Version != 2 {
		t.Errorf("Update() Version = %d, want 2", updated.Version)
	}

	// A second update increments again.
	name := "Renamed"
	updated2, err := svc.Update(ctx, created.ID, domain.FlagPatch{Name: &name})
	if err != nil {
		t.Fatalf("second Update() unexpected error: %v", err)
	}
	if updated2.Version != 3 {
		t.Errorf("second Update() Version = %d, want 3", updated2.Version)
	}
	if updated2.Name != "Renamed" {
		t.Errorf("second Update() Name = %q, want %q", updated2.Name, "Renamed")
	}
	if !updated2.Enabled {
		t.Error("second Update() Enabled = false, want true (unchanged from prior update)")
	}
}

func TestFlagService_Update_NotFound(t *testing.T) {
	svc := NewFlagService(memory.NewFlagRepository(), memory.NewEnvironmentRepository())
	_, err := svc.Update(context.Background(), "missing", domain.FlagPatch{})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestFlagService_Update_ValidationOnRenameToEmpty(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	environments := memory.NewEnvironmentRepository()
	env := newEnvironmentForFlagTest(t, ctx, projects, environments)

	svc := NewFlagService(memory.NewFlagRepository(), environments)
	created, err := svc.Create(ctx, env.ID, CreateFlagInput{Key: "new-checkout", Name: "New Checkout"})
	if err != nil {
		t.Fatal(err)
	}

	empty := "   "
	_, err = svc.Update(ctx, created.ID, domain.FlagPatch{Name: &empty})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("Update() error = %v, want ErrValidation", err)
	}
}

func TestFlagService_ListByEnvironment(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	environments := memory.NewEnvironmentRepository()
	env := newEnvironmentForFlagTest(t, ctx, projects, environments)

	svc := NewFlagService(memory.NewFlagRepository(), environments)
	if _, err := svc.Create(ctx, env.ID, CreateFlagInput{Key: "a", Name: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, env.ID, CreateFlagInput{Key: "b", Name: "B"}); err != nil {
		t.Fatal(err)
	}

	list, err := svc.ListByEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("ListByEnvironment() unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListByEnvironment() returned %d flags, want 2", len(list))
	}
}

func TestFlagService_ListByEnvironment_EnvironmentNotFound(t *testing.T) {
	svc := NewFlagService(memory.NewFlagRepository(), memory.NewEnvironmentRepository())
	_, err := svc.ListByEnvironment(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ListByEnvironment() error = %v, want ErrNotFound", err)
	}
}
