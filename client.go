// Package flagship is the Flagship Go SDK. It lets an application evaluate
// boolean feature flags locally, without an HTTP request per evaluation.
//
// A Client holds an immutable configuration Snapshot. Evaluations read
// from whichever snapshot is currently installed; installing a new
// snapshot replaces it atomically (see Client.UpdateSnapshot).
package flagship

import (
	"errors"
	"sync/atomic"
)

// ErrVersionGap is returned by Client.ApplyEvent when the event cannot be
// safely applied on top of the client's current snapshot: either the
// client has no snapshot yet, or the event's version is more than one
// past the client's current version. Either way, the caller must recover
// by fetching and installing a full, authoritative snapshot (e.g.
// FetchSnapshot + UpdateSnapshot) rather than guessing at the missing
// state.
var ErrVersionGap = errors.New("flagship: event does not sequentially follow the client's current snapshot")

// ErrWrongEnvironment is returned by Client.ApplyEvent when the event's
// Environment doesn't match the client's current snapshot. This signals
// caller misuse (an event routed to the wrong client), not a normal gap
// in the protocol.
var ErrWrongEnvironment = errors.New("flagship: event environment does not match the client's current snapshot")

// Client evaluates feature flags against a locally held configuration
// snapshot. It is safe for concurrent use by multiple goroutines.
type Client struct {
	snapshot atomic.Pointer[Snapshot]
}

// NewClient returns a Client holding snapshot as its initial configuration.
// snapshot may be nil, in which case the client starts with no
// configuration and every evaluation returns the caller-provided default.
func NewClient(snapshot *Snapshot) *Client {
	c := &Client{}
	if snapshot != nil {
		c.snapshot.Store(snapshot)
	}
	return c
}

// Bool evaluates the boolean flag identified by key.
//
// If the client's current snapshot contains key, its configured value is
// returned. Otherwise, or if the client has no snapshot at all,
// defaultValue is returned. defaultValue is never ignored.
func (c *Client) Bool(key string, defaultValue bool) bool {
	snapshot := c.snapshot.Load()
	if snapshot == nil {
		return defaultValue
	}
	if enabled, ok := snapshot.flag(key); ok {
		return enabled
	}
	return defaultValue
}

// UpdateSnapshot installs next as the client's current configuration if
// next is newer than what's currently installed (or if the client has no
// snapshot yet), and reports whether it was applied.
//
// An incoming snapshot with an equal or older version than the current one
// is rejected as a no-op rather than an error, since callers (a full
// bootstrap fetch, or recovery after ApplyEvent reports ErrVersionGap) may
// reasonably fetch a snapshot that turns out to already be current.
//
// UpdateSnapshot exists for a future configuration-sync mechanism to
// install new configuration as it arrives; most application code only
// needs Bool.
func (c *Client) UpdateSnapshot(next *Snapshot) bool {
	if next == nil {
		return false
	}
	for {
		current := c.snapshot.Load()
		if current != nil && next.version <= current.version {
			return false
		}
		if c.snapshot.CompareAndSwap(current, next) {
			return true
		}
	}
}

// ApplyEvent attempts to advance the client by exactly one configuration
// event, per the versioned sync protocol (see ADR-0004): events are
// applied strictly sequentially, and any gap is left for the caller to
// recover rather than guessed at.
//
//   - If the client has no snapshot yet, or e.Version is more than one
//     past the client's current version, ApplyEvent returns
//     ErrVersionGap without changing state. The caller must recover by
//     fetching and installing a full snapshot instead.
//   - If e.Version is less than or equal to the client's current
//     version, e is a stale or duplicate event: it's silently ignored,
//     and ApplyEvent returns (false, nil).
//   - If e.Environment doesn't match the client's current snapshot,
//     ApplyEvent returns ErrWrongEnvironment without changing state.
//   - Otherwise e is applied atomically and ApplyEvent returns (true, nil).
func (c *Client) ApplyEvent(e ConfigEvent) (applied bool, err error) {
	for {
		current := c.snapshot.Load()
		if current == nil || e.Version > current.version+1 {
			return false, ErrVersionGap
		}
		if current.environment != e.Environment {
			return false, ErrWrongEnvironment
		}
		if e.Version <= current.version {
			return false, nil
		}
		next := current.nextSnapshot(e)
		if c.snapshot.CompareAndSwap(current, next) {
			return true, nil
		}
	}
}
