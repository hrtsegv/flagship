package flagship

// ConfigEventType identifies what kind of change a ConfigEvent represents.
type ConfigEventType string

const (
	EventFlagCreated ConfigEventType = "flag_created"
	EventFlagUpdated ConfigEventType = "flag_updated"
	EventFlagDeleted ConfigEventType = "flag_deleted"
)

// ConfigEvent is one versioned, incremental change to an environment's
// configuration, as delivered by the SSE push transport. Version
// doubles as the event's identity: applying the events for an environment
// in strictly increasing version order, starting from an empty
// configuration, always produces the same result — see
// Client.ApplyEvent.
type ConfigEvent struct {
	Environment string // matches Snapshot.Environment(); a key, not an ID
	Version     int    // the version applying this event produces
	Type        ConfigEventType
	FlagKey     string
	Enabled     bool // meaningful for EventFlagCreated/EventFlagUpdated only
}
