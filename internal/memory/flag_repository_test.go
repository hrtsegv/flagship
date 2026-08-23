package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/hrtsegv/flagship/internal/domain"
)

func TestFlagRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout", Version: 1}
	created, err := repo.Create(ctx, f)
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if created != f {
		t.Errorf("Create() = %+v, want %+v", created, f)
	}
}

func TestFlagRepository_Create_DuplicateKeyWithinEnvironment(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f1 := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout"}
	f2 := domain.Flag{ID: "2", EnvironmentID: "e1", Key: "new-checkout"}

	if _, err := repo.Create(ctx, f1); err != nil {
		t.Fatalf("first Create() unexpected error: %v", err)
	}
	_, err := repo.Create(ctx, f2)
	if !errors.Is(err, domain.ErrDuplicateKey) {
		t.Fatalf("second Create() error = %v, want ErrDuplicateKey", err)
	}
}

func TestFlagRepository_Create_SameKeyDifferentEnvironmentsAllowed(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f1 := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout"}
	f2 := domain.Flag{ID: "2", EnvironmentID: "e2", Key: "new-checkout"}

	if _, err := repo.Create(ctx, f1); err != nil {
		t.Fatalf("Create() for e1 unexpected error: %v", err)
	}
	if _, err := repo.Create(ctx, f2); err != nil {
		t.Fatalf("Create() for e2 unexpected error: %v", err)
	}
}

func TestFlagRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout"}
	if _, err := repo.Create(ctx, f); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, "1")
	if err != nil {
		t.Fatalf("GetByID() unexpected error: %v", err)
	}
	if got.Key != "new-checkout" {
		t.Errorf("GetByID() Key = %q, want %q", got.Key, "new-checkout")
	}
}

func TestFlagRepository_GetByID_NotFound(t *testing.T) {
	repo := NewFlagRepository()
	_, err := repo.GetByID(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestFlagRepository_ListByEnvironment(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f1 := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}
	f2 := domain.Flag{ID: "2", EnvironmentID: "e1", Key: "b"}
	f3 := domain.Flag{ID: "3", EnvironmentID: "e2", Key: "a"}

	for _, f := range []domain.Flag{f1, f2, f3} {
		if _, err := repo.Create(ctx, f); err != nil {
			t.Fatal(err)
		}
	}

	list, err := repo.ListByEnvironment(ctx, "e1")
	if err != nil {
		t.Fatalf("ListByEnvironment() unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListByEnvironment() returned %d flags, want 2", len(list))
	}
}

func TestFlagRepository_Update_IncrementsVersion(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout", Enabled: false, Version: 1}
	if _, err := repo.Create(ctx, f); err != nil {
		t.Fatal(err)
	}

	enabled := true
	updated, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if !updated.Enabled {
		t.Errorf("Update() Enabled = false, want true")
	}
	if updated.Version != 2 {
		t.Errorf("Update() Version = %d, want 2", updated.Version)
	}

	stored, err := repo.GetByID(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != 2 {
		t.Errorf("stored Version = %d, want 2", stored.Version)
	}
}

func TestFlagRepository_Update_NotFound(t *testing.T) {
	repo := NewFlagRepository()
	_, err := repo.Update(context.Background(), "missing", domain.FlagPatch{})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestFlagRepository_Update_ConcurrentIncrementsAreSerialized(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	f := domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout", Version: 1}
	if _, err := repo.Create(ctx, f); err != nil {
		t.Fatal(err)
	}

	const updates = 100
	var wg sync.WaitGroup
	for i := 0; i < updates; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.Update(ctx, "1", domain.FlagPatch{}); err != nil {
				t.Errorf("Update() unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	final, err := repo.GetByID(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	want := 1 + updates
	if final.Version != want {
		t.Fatalf("final Version = %d, want %d (no updates should be lost)", final.Version, want)
	}
}

func TestFlagRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout"}); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, "1"); err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}

	if _, err := repo.GetByID(ctx, "1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID() after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestFlagRepository_Delete_NotFound(t *testing.T) {
	repo := NewFlagRepository()
	err := repo.Delete(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestFlagRepository_Delete_KeyBecomesReusable(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "new-checkout"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, "1"); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.Create(ctx, domain.Flag{ID: "2", EnvironmentID: "e1", Key: "new-checkout"}); err != nil {
		t.Fatalf("Create() after Delete() unexpected error: %v, want the key to be reusable", err)
	}
}

func TestFlagRepository_Delete_IncrementsConfigVersionAndScopesSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, domain.Flag{ID: "2", EnvironmentID: "e1", Key: "b"}); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, "1"); err != nil {
		t.Fatal(err)
	}

	version, flags, err := repo.Snapshot(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("version after 2 creates + 1 delete = %d, want 3", version)
	}
	if len(flags) != 1 || flags[0].Key != "b" {
		t.Fatalf("flags after delete = %+v, want only %q", flags, "b")
	}
}

func TestFlagRepository_Snapshot_EmptyEnvironment(t *testing.T) {
	repo := NewFlagRepository()

	version, flags, err := repo.Snapshot(context.Background(), "e1")
	if err != nil {
		t.Fatalf("Snapshot() unexpected error: %v", err)
	}
	if version != 0 {
		t.Errorf("Snapshot() version = %d, want 0 for an environment with no flags yet", version)
	}
	if len(flags) != 0 {
		t.Errorf("Snapshot() flags = %v, want empty", flags)
	}
}

func TestFlagRepository_Snapshot_IncrementsOnCreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	version, _, err := repo.Snapshot(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("version after first create = %d, want 1", version)
	}

	if _, err := repo.Create(ctx, domain.Flag{ID: "2", EnvironmentID: "e1", Key: "b"}); err != nil {
		t.Fatal(err)
	}
	version, flags, err := repo.Snapshot(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("version after second create = %d, want 2", version)
	}
	if len(flags) != 2 {
		t.Fatalf("flags after second create = %d, want 2", len(flags))
	}

	enabled := true
	if _, err := repo.Update(ctx, "1", domain.FlagPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	version, _, err = repo.Snapshot(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("version after update = %d, want 3", version)
	}
}

func TestFlagRepository_Snapshot_ScopedToEnvironment(t *testing.T) {
	ctx := context.Background()
	repo := NewFlagRepository()

	if _, err := repo.Create(ctx, domain.Flag{ID: "1", EnvironmentID: "e1", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, domain.Flag{ID: "2", EnvironmentID: "e2", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, domain.Flag{ID: "3", EnvironmentID: "e2", Key: "b"}); err != nil {
		t.Fatal(err)
	}

	v1, f1, err := repo.Snapshot(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if v1 != 1 || len(f1) != 1 {
		t.Fatalf("e1 snapshot = (version %d, %d flags), want (1, 1)", v1, len(f1))
	}

	v2, f2, err := repo.Snapshot(ctx, "e2")
	if err != nil {
		t.Fatal(err)
	}
	if v2 != 2 || len(f2) != 2 {
		t.Fatalf("e2 snapshot = (version %d, %d flags), want (2, 2)", v2, len(f2))
	}
}

func TestProjectRepository_Create_ConcurrentDistinctKeys(t *testing.T) {
	ctx := context.Background()
	repo := NewProjectRepository()

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := domain.Project{ID: fmt.Sprintf("id-%d", i), Key: fmt.Sprintf("key-%d", i)}
			if _, err := repo.Create(ctx, p); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("unexpected error creating project with distinct key: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != n {
		t.Fatalf("List() returned %d projects, want %d", len(list), n)
	}
}
