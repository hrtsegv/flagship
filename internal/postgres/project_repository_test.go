package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hrtsegv/flagship/internal/domain"
)

func TestProjectRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := NewProjectRepository(testPool(t))

	now := time.Now().UTC().Truncate(time.Microsecond)
	p := domain.Project{ID: "1", Key: "ecommerce", Name: "Ecommerce", CreatedAt: now, UpdatedAt: now}
	created, err := repo.Create(ctx, p)
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if created != p {
		t.Errorf("Create() = %+v, want %+v", created, p)
	}
}

func TestProjectRepository_Create_DuplicateKey(t *testing.T) {
	ctx := context.Background()
	repo := NewProjectRepository(testPool(t))

	p1 := domain.Project{ID: "1", Key: "ecommerce"}
	p2 := domain.Project{ID: "2", Key: "ecommerce"}

	if _, err := repo.Create(ctx, p1); err != nil {
		t.Fatalf("first Create() unexpected error: %v", err)
	}
	_, err := repo.Create(ctx, p2)
	if !errors.Is(err, domain.ErrDuplicateKey) {
		t.Fatalf("second Create() error = %v, want ErrDuplicateKey", err)
	}
}

func TestProjectRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := NewProjectRepository(testPool(t))

	p := domain.Project{ID: "1", Key: "ecommerce", Name: "Ecommerce"}
	if _, err := repo.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, "1")
	if err != nil {
		t.Fatalf("GetByID() unexpected error: %v", err)
	}
	if got.Key != "ecommerce" {
		t.Errorf("GetByID() Key = %q, want %q", got.Key, "ecommerce")
	}
}

func TestProjectRepository_GetByID_NotFound(t *testing.T) {
	repo := NewProjectRepository(testPool(t))
	_, err := repo.GetByID(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestProjectRepository_List(t *testing.T) {
	ctx := context.Background()
	repo := NewProjectRepository(testPool(t))

	for _, p := range []domain.Project{{ID: "1", Key: "a"}, {ID: "2", Key: "b"}} {
		if _, err := repo.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List() returned %d projects, want 2", len(list))
	}
}

func TestProjectRepository_Create_ConcurrentDistinctKeys(t *testing.T) {
	ctx := context.Background()
	repo := NewProjectRepository(testPool(t))

	const n = 20
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
