package domain

import "time"

// Flag is a boolean, named runtime decision that belongs to exactly one
// environment. It is uniquely identified by EnvironmentID + Key.
type Flag struct {
	ID            string
	EnvironmentID string
	Key           string
	Name          string
	Description   string
	Enabled       bool
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// FlagPatch describes an update to a subset of a flag's mutable fields.
// A nil field is left unchanged.
type FlagPatch struct {
	Name        *string
	Description *string
	Enabled     *bool
}

// Apply mutates the flag with any fields set on patch and increments the
// flag's version. Updating a flag must always increment its version, even
// if the patch is empty, since a version bump signals a new configuration
// state to callers.
func (f *Flag) Apply(patch FlagPatch, now time.Time) {
	if patch.Name != nil {
		f.Name = *patch.Name
	}
	if patch.Description != nil {
		f.Description = *patch.Description
	}
	if patch.Enabled != nil {
		f.Enabled = *patch.Enabled
	}
	f.Version++
	f.UpdatedAt = now
}
