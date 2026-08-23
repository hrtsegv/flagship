package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/hrtsegv/flagship/internal/domain"
)

// maxRetainedEventsPerEnvironment caps each environment's event history.
// Once it's exceeded the oldest events are dropped, and a client that has
// fallen further behind than this can no longer be brought up to date by
// replay — EventsSince reports that, and the client recovers from a full
// snapshot instead. Retaining everything forever would be the only
// alternative, which an in-memory store can't promise.
const maxRetainedEventsPerEnvironment = 1000

// FlagRepository is an in-memory store of flags, safe for concurrent use.
type FlagRepository struct {
	mu             sync.RWMutex
	byID           map[string]domain.Flag
	keys           map[string]struct{}             // "environmentID\x00key" -> exists
	configVersions map[string]int                  // environmentID -> configuration version
	events         map[string][]domain.ConfigEvent // environmentID -> ordered event history
}

// NewFlagRepository returns an empty FlagRepository.
func NewFlagRepository() *FlagRepository {
	return &FlagRepository{
		byID:           make(map[string]domain.Flag),
		keys:           make(map[string]struct{}),
		configVersions: make(map[string]int),
		events:         make(map[string][]domain.ConfigEvent),
	}
}

func flagKeyIndex(environmentID, key string) string {
	return environmentID + "\x00" + key
}

// recordEvent increments environmentID's configuration version and appends
// the matching event, trimming the oldest history past the retention cap.
// The caller must hold r.mu for writing: bumping the version and recording
// what changed at that version are deliberately one indivisible step, so
// no observer can ever see a version whose event is missing (or vice
// versa).
func (r *FlagRepository) recordEvent(environmentID string, eventType domain.ConfigEventType, flagKey string, enabled bool) {
	r.configVersions[environmentID]++

	events := append(r.events[environmentID], domain.ConfigEvent{
		EnvironmentID: environmentID,
		Version:       r.configVersions[environmentID],
		Type:          eventType,
		FlagKey:       flagKey,
		Enabled:       enabled,
	})
	if len(events) > maxRetainedEventsPerEnvironment {
		events = events[len(events)-maxRetainedEventsPerEnvironment:]
	}
	r.events[environmentID] = events
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
	r.recordEvent(flag.EnvironmentID, domain.ConfigEventFlagCreated, flag.Key, flag.Enabled)
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
	// An event is recorded even when the patch touched only fields the SDK
	// doesn't evaluate (name, description): the version bumped regardless,
	// and skipping the event would leave SDKs unable to reach that version
	// without a full snapshot.
	r.recordEvent(f.EnvironmentID, domain.ConfigEventFlagUpdated, f.Key, f.Enabled)
	return f, nil
}

// Delete removes the flag with the given ID, returning domain.ErrNotFound
// if it does not exist. Its key becomes available for reuse within the
// same environment, and the environment's configuration version is
// incremented — deletion is a mutating event like Create and Update.
func (r *FlagRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("%w: flag %q", domain.ErrNotFound, id)
	}

	delete(r.byID, id)
	delete(r.keys, flagKeyIndex(f.EnvironmentID, f.Key))
	r.recordEvent(f.EnvironmentID, domain.ConfigEventFlagDeleted, f.Key, false)
	return nil
}

// EventsSince returns environmentID's events after sinceVersion, in
// version order, for a client resuming from that version.
//
// replayable reports whether the returned events are sufficient to bring
// such a client fully up to date. It is false when the events the client
// missed have already been trimmed from the retained history, or when
// sinceVersion is ahead of what this repository has — in both cases the
// caller cannot safely resume and must fall back to a full snapshot. A
// client that is already current is replayable with no events.
func (r *FlagRepository) EventsSince(ctx context.Context, environmentID string, sinceVersion int) (events []domain.ConfigEvent, replayable bool, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	current := r.configVersions[environmentID]
	if sinceVersion > current {
		return nil, false, nil
	}
	if sinceVersion == current {
		return nil, true, nil
	}

	retained := r.events[environmentID]
	if len(retained) == 0 || retained[0].Version > sinceVersion+1 {
		return nil, false, nil
	}

	out := make([]domain.ConfigEvent, 0, current-sinceVersion)
	for _, e := range retained {
		if e.Version > sinceVersion {
			out = append(out, e)
		}
	}
	return out, true, nil
}

// Snapshot returns the current configuration version for environmentID
// together with its flags, read under a single lock so the two are always
// consistent with each other (a concurrent Create/Update can never be
// reflected in one but not the other).
func (r *FlagRepository) Snapshot(ctx context.Context, environmentID string) (version int, flags []domain.Flag, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domain.Flag, 0)
	for _, f := range r.byID {
		if f.EnvironmentID == environmentID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return r.configVersions[environmentID], out, nil
}
