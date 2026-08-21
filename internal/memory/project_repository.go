// Package memory provides an in-memory, concurrency-safe implementation of
// the repository interfaces used by the application layer.
package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/hrtsegv/flagship/internal/domain"
)

// ProjectRepository is an in-memory store of projects, safe for concurrent
// use.
type ProjectRepository struct {
	mu   sync.RWMutex
	byID map[string]domain.Project
	keys map[string]struct{}
}

// NewProjectRepository returns an empty ProjectRepository.
func NewProjectRepository() *ProjectRepository {
	return &ProjectRepository{
		byID: make(map[string]domain.Project),
		keys: make(map[string]struct{}),
	}
}

// Create stores project, rejecting it with domain.ErrDuplicateKey if a
// project with the same key already exists. The key uniqueness check and
// insert happen atomically under a single lock.
func (r *ProjectRepository) Create(ctx context.Context, project domain.Project) (domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.keys[project.Key]; exists {
		return domain.Project{}, fmt.Errorf("%w: project key %q", domain.ErrDuplicateKey, project.Key)
	}

	r.byID[project.ID] = project
	r.keys[project.Key] = struct{}{}
	return project, nil
}

// List returns all projects ordered by creation time.
func (r *ProjectRepository) List(ctx context.Context) ([]domain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domain.Project, 0, len(r.byID))
	for _, p := range r.byID {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// GetByID returns the project with the given ID, or domain.ErrNotFound if
// it does not exist.
func (r *ProjectRepository) GetByID(ctx context.Context, id string) (domain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.byID[id]
	if !ok {
		return domain.Project{}, fmt.Errorf("%w: project %q", domain.ErrNotFound, id)
	}
	return p, nil
}
