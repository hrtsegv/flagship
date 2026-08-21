package memory

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
	repo := NewProjectRepository()

	p := domain.Project{ID: "1", Key: "ecommerce", Name: "Ecommerce", CreatedAt: time.Now()}
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
	repo := NewProjectRepository()

	p1 := domain.Project{ID: "1", Key: "ecommerce", Name: "Ecommerce"}
	p2 := domain.Project{ID: "2", Key: "ecommerce", Name: "Ecommerce Duplicate"}

	if _, err := repo.Create(ctx, p1); err != nil {
		t.Fatalf("first Create() unexpected error: %v", err)
	}
	_, err := repo.Create(ctx, p2)
	if !errors.Is(err, domain.ErrDuplicateKey) {
		t.Fatalf("second Create() error = %v, want ErrDuplicateKey", err)
	}
}

func TestProjectRepository_GetByID_NotFound(t *testing.T) {
	repo := NewProjectRepository()
	_, err := repo.GetByID(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestProjectRepository_List_OrderedByCreation(t *testing.T) {
	ctx := context.Background()
	repo := NewProjectRepository()

	now := time.Now()
	p1 := domain.Project{ID: "1", Key: "a", CreatedAt: now}
	p2 := domain.Project{ID: "2", Key: "b", CreatedAt: now.Add(time.Second)}

	if _, err := repo.Create(ctx, p2); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, p1); err != nil {
		t.Fatal(err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(list) != 2 || list[0].ID != "1" || list[1].ID != "2" {
		t.Fatalf("List() = %+v, want [p1, p2] ordered by CreatedAt", list)
	}
}

func TestProjectRepository_Create_ConcurrentSameKey(t *testing.T) {
	ctx := context.Background()
	repo := NewProjectRepository()

	const attempts = 50
	var wg sync.WaitGroup
	successes := make(chan struct{}, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := domain.Project{ID: fmt.Sprintf("id-%d", i), Key: "same-key", Name: "Project"}
			if _, err := repo.Create(ctx, p); err == nil {
				successes <- struct{}{}
			}
		}(i)
	}
	wg.Wait()
	close(successes)

	count := 0
	for range successes {
		count++
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 successful Create() with the same key under concurrency, got %d", count)
	}
}
