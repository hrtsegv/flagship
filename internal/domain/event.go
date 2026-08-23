package domain

// ConfigEventType identifies what kind of change a ConfigEvent represents.
type ConfigEventType string

const (
	ConfigEventFlagCreated ConfigEventType = "flag_created"
	ConfigEventFlagUpdated ConfigEventType = "flag_updated"
	ConfigEventFlagDeleted ConfigEventType = "flag_deleted"
)

// ConfigEvent is one versioned change to an environment's configuration.
//
// Version is the environment's configuration version produced by applying
// this event, and doubles as the event's identity: the events for an
// environment form a single gap-free sequence, so replaying them in
// version order from an empty configuration always reconstructs the same
// state. Recording an event and incrementing the environment's version are
// the same operation, so a version can never mean two different things.
type ConfigEvent struct {
	EnvironmentID string
	Version       int
	Type          ConfigEventType
	FlagKey       string
	Enabled       bool // meaningful for created/updated events only
}
