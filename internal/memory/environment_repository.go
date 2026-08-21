package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/hrtsegv/flagship/internal/domain"
)

// EnvironmentRepository is an in-memory store of environments, safe for
// concurrent use.
type EnvironmentRepository struct {
	mu   sync.RWMutex
	byID map[string]domain.Environment
	keys map[string]struct{} // "projectID\x00key" -> exists
}

// NewEnvironmentRepository returns an empty EnvironmentRepository.
func NewEnvironmentRepository() *EnvironmentRepository {
	return &EnvironmentRepository{
		byID: make(map[string]domain.Environment),
		keys: make(map[string]struct{}),
	}
}

func environmentKeyIndex(projectID, key string) string {
	return projectID + "\x00" + key
}

// Create stores env, rejecting it with domain.ErrDuplicateKey if an
// environment with the same key already exists within the same project.
func (r *EnvironmentRepository) Create(ctx context.Context, env domain.Environment) (domain.Environment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	index := environmentKeyIndex(env.ProjectID, env.Key)
	if _, exists := r.keys[index]; exists {
		return domain.Environment{}, fmt.Errorf("%w: environment key %q", domain.ErrDuplicateKey, env.Key)
	}

	r.byID[env.ID] = env
	r.keys[index] = struct{}{}
	return env, nil
}

// ListByProject returns all environments belonging to projectID, ordered by
// creation time.
func (r *EnvironmentRepository) ListByProject(ctx context.Context, projectID string) ([]domain.Environment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domain.Environment, 0)
	for _, e := range r.byID {
		if e.ProjectID == projectID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// GetByID returns the environment with the given ID, or domain.ErrNotFound
// if it does not exist.
func (r *EnvironmentRepository) GetByID(ctx context.Context, id string) (domain.Environment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.byID[id]
	if !ok {
		return domain.Environment{}, fmt.Errorf("%w: environment %q", domain.ErrNotFound, id)
	}
	return e, nil
}
