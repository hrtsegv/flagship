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

type configurationFlagResponse struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
}

type configurationResponse struct {
	Version     int                         `json:"version"`
	Environment string                      `json:"environment"`
	Flags       []configurationFlagResponse `json:"flags"`
}

func toConfigurationResponse(env domain.Environment, version int, flags []domain.Flag) configurationResponse {
	resp := configurationResponse{
		Version:     version,
		Environment: env.Key,
		Flags:       make([]configurationFlagResponse, 0, len(flags)),
	}
	for _, f := range flags {
		resp.Flags = append(resp.Flags, configurationFlagResponse{Key: f.Key, Enabled: f.Enabled})
	}
	return resp
}

// configurationResetPayload requests a full snapshot without advancing the
// client's version or claiming that a mutation was applied.
type configurationResetPayload struct {
	Environment string `json:"environment"`
}

// configEventPayload is the JSON shape of one SSE configuration.updated
// event's data field.
type configEventPayload struct {
	Environment string `json:"environment"`
	Version     int    `json:"version"`
	Type        string `json:"type"`
	FlagKey     string `json:"flag_key"`
	Enabled     bool   `json:"enabled"`
}

func toConfigEventPayload(environmentKey string, e domain.ConfigEvent) configEventPayload {
	return configEventPayload{
		Environment: environmentKey,
		Version:     e.Version,
		Type:        string(e.Type),
		FlagKey:     e.FlagKey,
		Enabled:     e.Enabled,
	}
}
