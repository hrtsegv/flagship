package flagship

import "maps"

// Flag is a boolean feature flag as delivered to the SDK: just enough
// information to evaluate it locally.
type Flag struct {
	Key     string
	Enabled bool
}

// Snapshot is an immutable configuration snapshot for a single environment:
// a version plus the set of boolean flags known as of that version. Once
// constructed, a Snapshot is never mutated. Configuration changes are
// applied by installing a new Snapshot (see Client.UpdateSnapshot).
type Snapshot struct {
	environment string
	version     int
	flags       map[string]bool
}

// NewSnapshot builds a Snapshot for environment at version, from flags. The
// provided flags are copied, so the caller's slice can be reused or mutated
// afterward without affecting the snapshot.
func NewSnapshot(environment string, version int, flags []Flag) *Snapshot {
	m := make(map[string]bool, len(flags))
	for _, f := range flags {
		m[f.Key] = f.Enabled
	}
	return &Snapshot{environment: environment, version: version, flags: m}
}

// Environment returns the environment this snapshot was produced for.
func (s *Snapshot) Environment() string {
	return s.environment
}

// Version returns the snapshot's configuration version.
func (s *Snapshot) Version() int {
	return s.version
}

func (s *Snapshot) flag(key string) (enabled, ok bool) {
	enabled, ok = s.flags[key]
	return enabled, ok
}

// nextSnapshot returns a new Snapshot reflecting e applied on top of s: a
// copy of s's flags with e's one change made, at e.Version. The caller
// (Client.ApplyEvent) is responsible for only calling this when e.Version
// is exactly s.version+1 — nextSnapshot itself makes no sequencing
// decisions, it just applies the one delta it's given.
func (s *Snapshot) nextSnapshot(e ConfigEvent) *Snapshot {
	flags := make(map[string]bool, len(s.flags)+1)
	maps.Copy(flags, s.flags)

	switch e.Type {
	case EventFlagCreated, EventFlagUpdated:
		flags[e.FlagKey] = e.Enabled
	case EventFlagDeleted:
		delete(flags, e.FlagKey)
	}

	return &Snapshot{environment: s.environment, version: e.Version, flags: flags}
}
