package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hrtsegv/flagship/internal/domain"
)

// maxRetainedEventsPerEnvironment caps each environment's event history,
// mirroring internal/memory's retention window exactly so replay/gap
// behavior is identical regardless of backend. Once it's exceeded the
// oldest events are dropped, and a client that has fallen further behind
// than this recovers via a full snapshot instead of replay.
const maxRetainedEventsPerEnvironment = 1000

// subscriberBufferSize bounds how far a live subscriber can lag before its
// subscription is closed, matching internal/memory. The SDK reconnects
// and replays or fetches a snapshot without waiting for another mutation.
const subscriberBufferSize = 16

// FlagRepository is a PostgreSQL-backed store of flags. Durable state
// (flags, configuration versions, the event log) lives in the database;
// live SSE subscriber delivery stays in-process via Go channels for
// the supported single-instance deployment. Cross-instance live event
// delivery is outside the project scope.
type FlagRepository struct {
	pool *pgxpool.Pool

	// mu serializes every mutation's (DB transaction + local publish) and
	// every SubscribeFrom's (DB read + local subscriber registration)
	// against each other, so the replayed history and the first event the
	// live channel can ever deliver are always contiguous — no gap, no
	// duplicate — exactly like internal/memory's single-lock design. It's a
	// deliberate simplicity-over-throughput tradeoff: correct and
	// sufficient at single-instance scale, revisited only if load testing
	// shows it's actually a bottleneck.
	mu          sync.Mutex
	subscribers map[string]map[chan domain.ConfigEvent]struct{} // environmentID -> live subscribers
}

// NewFlagRepository returns a FlagRepository backed by pool.
func NewFlagRepository(pool *pgxpool.Pool) *FlagRepository {
	return &FlagRepository{
		pool:        pool,
		subscribers: make(map[string]map[chan domain.ConfigEvent]struct{}),
	}
}

// Create stores flag, rejecting it with domain.ErrDuplicateKey if a flag
// with the same key already exists within the same environment.
func (r *FlagRepository) Create(ctx context.Context, flag domain.Flag) (domain.Flag, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Flag{}, fmt.Errorf("postgres: begin create flag: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO flags (id, environment_id, key, name, description, enabled, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		flag.ID, flag.EnvironmentID, flag.Key, flag.Name, flag.Description, flag.Enabled, flag.Version, flag.CreatedAt, flag.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Flag{}, fmt.Errorf("%w: flag key %q", domain.ErrDuplicateKey, flag.Key)
		}
		return domain.Flag{}, fmt.Errorf("postgres: create flag: %w", err)
	}

	event, err := recordMutation(ctx, tx, flag.EnvironmentID, domain.ConfigEventFlagCreated, flag.Key, flag.Enabled)
	if err != nil {
		return domain.Flag{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Flag{}, fmt.Errorf("postgres: commit create flag: %w", err)
	}

	r.publishLocked(event)
	return flag, nil
}

// ListByEnvironment returns all flags belonging to environmentID, ordered
// by creation time.
func (r *FlagRepository) ListByEnvironment(ctx context.Context, environmentID string) ([]domain.Flag, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, environment_id, key, name, description, enabled, version, created_at, updated_at
		FROM flags
		WHERE environment_id = $1
		ORDER BY created_at`, environmentID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list flags: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Flag, 0)
	for rows.Next() {
		f, err := scanFlag(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan flag: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list flags: %w", err)
	}
	return out, nil
}

// GetByID returns the flag with the given ID, or domain.ErrNotFound if it
// does not exist.
func (r *FlagRepository) GetByID(ctx context.Context, id string) (domain.Flag, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, environment_id, key, name, description, enabled, version, created_at, updated_at
		FROM flags
		WHERE id = $1`, id)

	f, err := scanFlag(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Flag{}, fmt.Errorf("%w: flag %q", domain.ErrNotFound, id)
		}
		return domain.Flag{}, fmt.Errorf("postgres: get flag: %w", err)
	}
	return f, nil
}

// Update applies patch to the flag with the given ID and persists the
// result, returning domain.ErrNotFound if it does not exist.
func (r *FlagRepository) Update(ctx context.Context, id string, patch domain.FlagPatch) (domain.Flag, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Flag{}, fmt.Errorf("postgres: begin update flag: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// FOR UPDATE because this is a read-modify-write: the row is read here
	// and written below off the value read. r.mu already serializes writers
	// within this process, but that's an in-process lock held for the
	// channel publish -- it can't order writes issued by another process
	// against the same database, and without the row lock two of those
	// could both read version N and both write N+1, losing an increment.
	f, err := scanFlag(tx.QueryRow(ctx, `
		SELECT id, environment_id, key, name, description, enabled, version, created_at, updated_at
		FROM flags
		WHERE id = $1
		FOR UPDATE`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Flag{}, fmt.Errorf("%w: flag %q", domain.ErrNotFound, id)
		}
		return domain.Flag{}, fmt.Errorf("postgres: get flag for update: %w", err)
	}

	f.Apply(patch, time.Now().UTC())

	_, err = tx.Exec(ctx, `
		UPDATE flags
		SET name = $1, description = $2, enabled = $3, version = $4, updated_at = $5
		WHERE id = $6`,
		f.Name, f.Description, f.Enabled, f.Version, f.UpdatedAt, f.ID)
	if err != nil {
		return domain.Flag{}, fmt.Errorf("postgres: update flag: %w", err)
	}

	// An event is recorded even when the patch touched only fields the SDK
	// doesn't evaluate (name, description): the version bumped regardless,
	// and skipping the event would leave SDKs unable to reach that version
	// without a full snapshot.
	event, err := recordMutation(ctx, tx, f.EnvironmentID, domain.ConfigEventFlagUpdated, f.Key, f.Enabled)
	if err != nil {
		return domain.Flag{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Flag{}, fmt.Errorf("postgres: commit update flag: %w", err)
	}

	r.publishLocked(event)
	return f, nil
}

// Delete removes the flag with the given ID, returning domain.ErrNotFound
// if it does not exist. Its key becomes available for reuse within the
// same environment (enforced by the flags table's UNIQUE(environment_id,
// key) constraint once the row is gone), and the environment's
// configuration version is incremented like any other mutation.
func (r *FlagRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin delete flag: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// FOR UPDATE for the same reason as Update: without it, two concurrent
	// deletes could both read the row and both proceed, and the second
	// would record a flag_deleted event (and bump the version) for a flag
	// that was already gone. With it, the second waits, then sees no row
	// and reports ErrNotFound.
	var environmentID, key string
	err = tx.QueryRow(ctx, `SELECT environment_id, key FROM flags WHERE id = $1 FOR UPDATE`, id).Scan(&environmentID, &key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: flag %q", domain.ErrNotFound, id)
		}
		return fmt.Errorf("postgres: get flag for delete: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM flags WHERE id = $1`, id); err != nil {
		return fmt.Errorf("postgres: delete flag: %w", err)
	}

	event, err := recordMutation(ctx, tx, environmentID, domain.ConfigEventFlagDeleted, key, false)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit delete flag: %w", err)
	}

	r.publishLocked(event)
	return nil
}

// Snapshot returns the current configuration version for environmentID
// together with its flags, read inside a repeatable-read transaction so the
// two are always consistent with each other (a concurrent Create/Update can
// never be reflected in one but not the other).
func (r *FlagRepository) Snapshot(ctx context.Context, environmentID string) (version int, flags []domain.Flag, err error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return 0, nil, fmt.Errorf("postgres: begin snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := tx.QueryRow(ctx, `SELECT config_version FROM environments WHERE id = $1`, environmentID).Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, make([]domain.Flag, 0), nil
		}
		return 0, nil, fmt.Errorf("postgres: read config version: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT id, environment_id, key, name, description, enabled, version, created_at, updated_at
		FROM flags
		WHERE environment_id = $1
		ORDER BY created_at`, environmentID)
	if err != nil {
		return 0, nil, fmt.Errorf("postgres: list flags for snapshot: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Flag, 0)
	for rows.Next() {
		f, err := scanFlag(rows)
		if err != nil {
			return 0, nil, fmt.Errorf("postgres: scan flag: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("postgres: list flags for snapshot: %w", err)
	}

	return version, out, nil
}

// SubscribeFrom atomically subscribes to environmentID's live event feed
// and returns the events since sinceVersion as of the moment of
// subscription. Doing both under r.mu means the returned history and the
// first event the live channel can ever deliver are contiguous.
//
// The caller must call unsubscribe when done (typically via defer).
func (r *FlagRepository) SubscribeFrom(ctx context.Context, environmentID string, sinceVersion int) (events []domain.ConfigEvent, replayable bool, live <-chan domain.ConfigEvent, unsubscribe func()) {
	r.mu.Lock()
	defer r.mu.Unlock()

	events, replayable = r.eventsSince(ctx, environmentID, sinceVersion)

	ch := make(chan domain.ConfigEvent, subscriberBufferSize)
	if r.subscribers[environmentID] == nil {
		r.subscribers[environmentID] = make(map[chan domain.ConfigEvent]struct{})
	}
	r.subscribers[environmentID][ch] = struct{}{}

	unsubscribe = func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if _, ok := r.subscribers[environmentID][ch]; ok {
			delete(r.subscribers[environmentID], ch)
			close(ch)
		}
		if len(r.subscribers[environmentID]) == 0 {
			delete(r.subscribers, environmentID)
		}
	}

	return events, replayable, ch, unsubscribe
}

// eventsSince returns environmentID's events after sinceVersion, in version
// order, and whether they're sufficient to bring a client resuming from
// that version fully up to date. It's false when the events the client
// missed have already been trimmed from the retained history, or when
// sinceVersion is ahead of what this repository has. The caller must hold
// r.mu.
func (r *FlagRepository) eventsSince(ctx context.Context, environmentID string, sinceVersion int) (events []domain.ConfigEvent, replayable bool) {
	var current int
	err := r.pool.QueryRow(ctx, `SELECT config_version FROM environments WHERE id = $1`, environmentID).Scan(&current)
	if err != nil {
		// Unknown environment: nothing to replay, and the caller (the
		// application layer) is expected to have already verified the
		// environment exists before subscribing.
		return nil, false
	}
	if sinceVersion > current {
		return nil, false
	}
	if sinceVersion == current {
		return nil, true
	}

	var oldestRetained *int
	if err := r.pool.QueryRow(ctx, `SELECT MIN(version) FROM config_events WHERE environment_id = $1`, environmentID).Scan(&oldestRetained); err != nil {
		return nil, false
	}
	if oldestRetained == nil || *oldestRetained > sinceVersion+1 {
		return nil, false
	}

	rows, err := r.pool.Query(ctx, `
		SELECT environment_id, version, type, flag_key, enabled
		FROM config_events
		WHERE environment_id = $1 AND version > $2
		ORDER BY version`, environmentID, sinceVersion)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	out := make([]domain.ConfigEvent, 0, current-sinceVersion)
	for rows.Next() {
		var e domain.ConfigEvent
		if err := rows.Scan(&e.EnvironmentID, &e.Version, &e.Type, &e.FlagKey, &e.Enabled); err != nil {
			return nil, false
		}
		out = append(out, e)
	}
	if rows.Err() != nil {
		return nil, false
	}
	return out, true
}

// publishLocked delivers event to every live subscriber for its
// environment, closing and removing any subscriber whose buffer is full
// rather than blocking. The caller must hold r.mu.
func (r *FlagRepository) publishLocked(event domain.ConfigEvent) {
	for ch := range r.subscribers[event.EnvironmentID] {
		select {
		case ch <- event:
		default:
			close(ch)
			delete(r.subscribers[event.EnvironmentID], ch)
		}
	}
	if len(r.subscribers[event.EnvironmentID]) == 0 {
		delete(r.subscribers, event.EnvironmentID)
	}
}

// recordMutation increments environmentID's configuration version, records
// the matching event, and trims history past the retention cap — all
// within tx, so a reader can never observe a version whose event is
// missing (or vice versa).
func recordMutation(ctx context.Context, tx pgx.Tx, environmentID string, eventType domain.ConfigEventType, flagKey string, enabled bool) (domain.ConfigEvent, error) {
	var version int
	err := tx.QueryRow(ctx, `
		UPDATE environments SET config_version = config_version + 1
		WHERE id = $1
		RETURNING config_version`, environmentID).Scan(&version)
	if err != nil {
		return domain.ConfigEvent{}, fmt.Errorf("postgres: bump config version: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO config_events (environment_id, version, type, flag_key, enabled, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		environmentID, version, eventType, flagKey, enabled, time.Now().UTC())
	if err != nil {
		return domain.ConfigEvent{}, fmt.Errorf("postgres: record config event: %w", err)
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM config_events
		WHERE environment_id = $1 AND version <= $2`,
		environmentID, version-maxRetainedEventsPerEnvironment)
	if err != nil {
		return domain.ConfigEvent{}, fmt.Errorf("postgres: trim config events: %w", err)
	}

	return domain.ConfigEvent{
		EnvironmentID: environmentID,
		Version:       version,
		Type:          eventType,
		FlagKey:       flagKey,
		Enabled:       enabled,
	}, nil
}

func scanFlag(row rowScanner) (domain.Flag, error) {
	var f domain.Flag
	err := row.Scan(&f.ID, &f.EnvironmentID, &f.Key, &f.Name, &f.Description, &f.Enabled, &f.Version, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}
