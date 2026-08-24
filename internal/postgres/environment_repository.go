package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hrtsegv/flagship/internal/domain"
)

// EnvironmentRepository is a PostgreSQL-backed store of environments.
type EnvironmentRepository struct {
	pool *pgxpool.Pool
}

// NewEnvironmentRepository returns an EnvironmentRepository backed by pool.
func NewEnvironmentRepository(pool *pgxpool.Pool) *EnvironmentRepository {
	return &EnvironmentRepository{pool: pool}
}

// Create stores env, rejecting it with domain.ErrDuplicateKey if an
// environment with the same key already exists within the same project.
func (r *EnvironmentRepository) Create(ctx context.Context, env domain.Environment) (domain.Environment, error) {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO environments (id, project_id, key, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		env.ID, env.ProjectID, env.Key, env.Name, env.CreatedAt, env.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Environment{}, fmt.Errorf("%w: environment key %q", domain.ErrDuplicateKey, env.Key)
		}
		return domain.Environment{}, fmt.Errorf("postgres: create environment: %w", err)
	}
	return env, nil
}

// ListByProject returns all environments belonging to projectID, ordered by
// creation time.
func (r *EnvironmentRepository) ListByProject(ctx context.Context, projectID string) ([]domain.Environment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, project_id, key, name, created_at, updated_at
		FROM environments
		WHERE project_id = $1
		ORDER BY created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list environments: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Environment, 0)
	for rows.Next() {
		e, err := scanEnvironment(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan environment: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list environments: %w", err)
	}
	return out, nil
}

// GetByID returns the environment with the given ID, or domain.ErrNotFound
// if it does not exist.
func (r *EnvironmentRepository) GetByID(ctx context.Context, id string) (domain.Environment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, project_id, key, name, created_at, updated_at
		FROM environments
		WHERE id = $1`, id)

	e, err := scanEnvironment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Environment{}, fmt.Errorf("%w: environment %q", domain.ErrNotFound, id)
		}
		return domain.Environment{}, fmt.Errorf("postgres: get environment: %w", err)
	}
	return e, nil
}

func scanEnvironment(row rowScanner) (domain.Environment, error) {
	var e domain.Environment
	err := row.Scan(&e.ID, &e.ProjectID, &e.Key, &e.Name, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}
