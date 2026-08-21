package httpapi

import (
	"time"

	"github.com/hrtsegv/flagship/internal/domain"
)

type createProjectRequest struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type projectResponse struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toProjectResponse(p domain.Project) projectResponse {
	return projectResponse{
		ID:          p.ID,
		Key:         p.Key,
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

type createEnvironmentRequest struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type environmentResponse struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toEnvironmentResponse(e domain.Environment) environmentResponse {
	return environmentResponse{
		ID:        e.ID,
		ProjectID: e.ProjectID,
		Key:       e.Key,
		Name:      e.Name,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

type createFlagRequest struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type updateFlagRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Enabled     *bool   `json:"enabled"`
}

type flagResponse struct {
	ID            string    `json:"id"`
	EnvironmentID string    `json:"environment_id"`
	Key           string    `json:"key"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Enabled       bool      `json:"enabled"`
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func toFlagResponse(f domain.Flag) flagResponse {
	return flagResponse{
		ID:            f.ID,
		EnvironmentID: f.EnvironmentID,
		Key:           f.Key,
		Name:          f.Name,
		Description:   f.Description,
		Enabled:       f.Enabled,
		Version:       f.Version,
		CreatedAt:     f.CreatedAt,
		UpdatedAt:     f.UpdatedAt,
	}
}
