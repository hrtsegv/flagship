package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/hrtsegv/flagship/internal/domain"
)

func TestEnvironmentRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := NewEnvironmentRepository()

	e := domain.Environment{ID: "1", ProjectID: "p1", Key: "production", Name: "Production"}
	created, err := repo.Create(ctx, e)
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if created != e {
		t.Errorf("Create() = %+v, want %+v", created, e)
	}
}

func TestEnvironmentRepository_Create_DuplicateKeyWithinProject(t *testing.T) {
	ctx := context.Background()
	repo := NewEnvironmentRepository()

	e1 := domain.Environment{ID: "1", ProjectID: "p1", Key: "production"}
	e2 := domain.Environment{ID: "2", ProjectID: "p1", Key: "production"}

	if _, err := repo.Create(ctx, e1); err != nil {
		t.Fatalf("first Create() unexpected error: %v", err)
	}
	_, err := repo.Create(ctx, e2)
	if !errors.Is(err, domain.ErrDuplicateKey) {
		t.Fatalf("second Create() error = %v, want ErrDuplicateKey", err)
	}
}

func TestEnvironmentRepository_Create_SameKeyDifferentProjectsAllowed(t *testing.T) {
	ctx := context.Background()
	repo := NewEnvironmentRepository()

	e1 := domain.Environment{ID: "1", ProjectID: "p1", Key: "production"}
	e2 := domain.Environment{ID: "2", ProjectID: "p2", Key: "production"}

	if _, err := repo.Create(ctx, e1); err != nil {
		t.Fatalf("Create() for p1 unexpected error: %v", err)
	}
	if _, err := repo.Create(ctx, e2); err != nil {
		t.Fatalf("Create() for p2 unexpected error: %v", err)
	}
}

func TestEnvironmentRepository_ListByProject(t *testing.T) {
	ctx := context.Background()
	repo := NewEnvironmentRepository()

	e1 := domain.Environment{ID: "1", ProjectID: "p1", Key: "production"}
	e2 := domain.Environment{ID: "2", ProjectID: "p1", Key: "staging"}
	e3 := domain.Environment{ID: "3", ProjectID: "p2", Key: "production"}

	for _, e := range []domain.Environment{e1, e2, e3} {
		if _, err := repo.Create(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	list, err := repo.ListByProject(ctx, "p1")
	if err != nil {
		t.Fatalf("ListByProject() unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListByProject() returned %d environments, want 2", len(list))
	}
}

func TestEnvironmentRepository_GetByID_NotFound(t *testing.T) {
	repo := NewEnvironmentRepository()
	_, err := repo.GetByID(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}
