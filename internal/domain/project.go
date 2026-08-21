package domain

import "time"

// Project is a logical grouping of applications/services.
type Project struct {
	ID          string
	Key         string
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
