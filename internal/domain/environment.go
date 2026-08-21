package domain

import "time"

// Environment is an isolated feature-flag configuration space inside a project.
type Environment struct {
	ID        string
	ProjectID string
	Key       string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}
