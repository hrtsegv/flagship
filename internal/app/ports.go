// Package app contains the application/use-case layer: it validates input,
// enforces cross-aggregate rules (e.g. an environment's project must
// exist), and orchestrates calls to the repository layer. It has no
// knowledge of HTTP or any other transport.
package app

import (
	"context"

	"github.com/hrtsegv/flagship/internal/domain"
)

// ProjectRepository persists and retrieves projects.
type ProjectRepository interface {
	Create(ctx context.Context, project domain.Project) (domain.Project, error)
	List(ctx context.Context) ([]domain.Project, error)
	GetByID(ctx context.Context, id string) (domain.Project, error)
}

// EnvironmentRepository persists and retrieves environments.
type EnvironmentRepository interface {
	Create(ctx context.Context, env domain.Environment) (domain.Environment, error)
	ListByProject(ctx context.Context, projectID string) ([]domain.Environment, error)
	GetByID(ctx context.Context, id string) (domain.Environment, error)
}

// FlagRepository persists and retrieves flags.
type FlagRepository interface {
	Create(ctx context.Context, flag domain.Flag) (domain.Flag, error)
	ListByEnvironment(ctx context.Context, environmentID string) ([]domain.Flag, error)
	GetByID(ctx context.Context, id string) (domain.Flag, error)
	Update(ctx context.Context, id string, patch domain.FlagPatch) (domain.Flag, error)
	Snapshot(ctx context.Context, environmentID string) (version int, flags []domain.Flag, err error)
}
