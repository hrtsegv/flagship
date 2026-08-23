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

func TestFlagService_Delete(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	environments := memory.NewEnvironmentRepository()
	env := newEnvironmentForFlagTest(t, ctx, projects, environments)

	svc := NewFlagService(memory.NewFlagRepository(), environments)
	flag, err := svc.Create(ctx, env.ID, CreateFlagInput{Key: "new-checkout", Name: "New Checkout"})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(ctx, flag.ID); err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}

	if _, err := svc.Get(ctx, flag.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get() after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestFlagService_Delete_NotFound(t *testing.T) {
	svc := NewFlagService(memory.NewFlagRepository(), memory.NewEnvironmentRepository())
	err := svc.Delete(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestFlagService_Stream(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	environments := memory.NewEnvironmentRepository()
	env := newEnvironmentForFlagTest(t, ctx, projects, environments)

	svc := NewFlagService(memory.NewFlagRepository(), environments)
	if _, err := svc.Create(ctx, env.ID, CreateFlagInput{Key: "new-checkout", Name: "New Checkout", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	sub, err := svc.Stream(ctx, env.ID, 0)
	if err != nil {
		t.Fatalf("Stream() unexpected error: %v", err)
	}
	defer sub.Unsubscribe()

	if sub.Environment.Key != "production" {
		t.Errorf("Environment.Key = %q, want %q", sub.Environment.Key, "production")
	}
	if !sub.Replayable {
		t.Fatal("Replayable = false, want true")
	}
	if len(sub.Events) != 1 || sub.Events[0].Type != domain.ConfigEventFlagCreated || sub.Events[0].FlagKey != "new-checkout" {
		t.Fatalf("Events = %+v, want a single flag_created event for new-checkout", sub.Events)
	}
	if sub.Live == nil {
		t.Error("Live channel is nil, want a usable subscription")
	}
}

func TestFlagService_Stream_EnvironmentNotFound(t *testing.T) {
	svc := NewFlagService(memory.NewFlagRepository(), memory.NewEnvironmentRepository())
	_, err := svc.Stream(context.Background(), "missing", 0)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Stream() error = %v, want ErrNotFound", err)
	}
}

func TestFlagService_Snapshot(t *testing.T) {
	ctx := context.Background()
	projects := memory.NewProjectRepository()
	environments := memory.NewEnvironmentRepository()
	env := newEnvironmentForFlagTest(t, ctx, projects, environments)

	svc := NewFlagService(memory.NewFlagRepository(), environments)
	if _, err := svc.Create(ctx, env.ID, CreateFlagInput{Key: "new-checkout", Name: "New Checkout", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	gotEnv, version, flags, err := svc.Snapshot(ctx, env.ID)
	if err != nil {
		t.Fatalf("Snapshot() unexpected error: %v", err)
	}
	if gotEnv.Key != "production" {
		t.Errorf("Snapshot() environment key = %q, want %q", gotEnv.Key, "production")
	}
	if version != 1 {
		t.Errorf("Snapshot() version = %d, want 1", version)
	}
	if len(flags) != 1 || flags[0].Key != "new-checkout" || !flags[0].Enabled {
		t.Errorf("Snapshot() flags = %+v, want one enabled new-checkout flag", flags)
	}
}

func TestFlagService_Snapshot_EnvironmentNotFound(t *testing.T) {
	svc := NewFlagService(memory.NewFlagRepository(), memory.NewEnvironmentRepository())
	_, _, _, err := svc.Snapshot(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Snapshot() error = %v, want ErrNotFound", err)
	}
}
