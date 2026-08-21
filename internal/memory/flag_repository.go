package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/hrtsegv/flagship/internal/domain"
)

// FlagRepository is an in-memory store of flags, safe for concurrent use.
type FlagRepository struct {
	mu   sync.RWMutex
	byID map[string]domain.Flag
	keys map[string]struct{} // "environmentID\x00key" -> exists
}

// NewFlagRepository returns an empty FlagRepository.
func NewFlagRepository() *FlagRepository {
	return &FlagRepository{
		byID: make(map[string]domain.Flag),
		keys: make(map[string]struct{}),
	}
}

func flagKeyIndex(environmentID, key string) string {
	return environmentID + "\x00" + key
}

// Create stores flag, rejecting it with domain.ErrDuplicateKey if a flag
// with the same key already exists within the same environment.
func (r *FlagRepository) Create(ctx context.Context, flag domain.Flag) (domain.Flag, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	index := flagKeyIndex(flag.EnvironmentID, flag.Key)
	if _, exists := r.keys[index]; exists {
		return domain.Flag{}, fmt.Errorf("%w: flag key %q", domain.ErrDuplicateKey, flag.Key)
	}

	r.byID[flag.ID] = flag
	r.keys[index] = struct{}{}
	return flag, nil
}

// ListByEnvironment returns all flags belonging to environmentID, ordered
// by creation time.
func (r *FlagRepository) ListByEnvironment(ctx context.Context, environmentID string) ([]domain.Flag, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domain.Flag, 0)
	for _, f := range r.byID {
		if f.EnvironmentID == environmentID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// GetByID returns the flag with the given ID, or domain.ErrNotFound if it
// does not exist.
func (r *FlagRepository) GetByID(ctx context.Context, id string) (domain.Flag, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	f, ok := r.byID[id]
	if !ok {
		return domain.Flag{}, fmt.Errorf("%w: flag %q", domain.ErrNotFound, id)
	}
	return f, nil
}

// Update applies patch to the flag with the given ID and persists the
// result, returning domain.ErrNotFound if it does not exist. The read,
// mutate, and write happen under a single lock so version increments never
// race.
func (r *FlagRepository) Update(ctx context.Context, id string, patch domain.FlagPatch) (domain.Flag, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.byID[id]
	if !ok {
		return domain.Flag{}, fmt.Errorf("%w: flag %q", domain.ErrNotFound, id)
	}

	f.Apply(patch, time.Now().UTC())
	r.byID[id] = f
	return f, nil
}
