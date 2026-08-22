// Package flagship is the Flagship Go SDK. It lets an application evaluate
// boolean feature flags locally, without an HTTP request per evaluation.
//
// A Client holds an immutable configuration Snapshot. Evaluations read
// from whichever snapshot is currently installed; installing a new
// snapshot replaces it atomically (see Client.UpdateSnapshot).
package flagship

import "sync/atomic"

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
// is rejected as a no-op rather than an error, since a future poller will
// call this on every poll and most polls won't observe a version change.
//
// UpdateSnapshot exists for a future configuration-sync mechanism
// (polling/SSE) to install new configuration as it arrives; most
// application code only needs Bool.
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
