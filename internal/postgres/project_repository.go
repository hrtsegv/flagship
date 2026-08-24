package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hrtsegv/flagship/internal/domain"
)

// ProjectRepository is a PostgreSQL-backed store of projects.
type ProjectRepository struct {
	pool *pgxpool.Pool
}

// NewProjectRepository returns a ProjectRepository backed by pool.
func NewProjectRepository(pool *pgxpool.Pool) *ProjectRepository {
	return &ProjectRepository{pool: pool}
}

// Create stores project, rejecting it with domain.ErrDuplicateKey if a
// project with the same key already exists.
func (r *ProjectRepository) Create(ctx context.Context, project domain.Project) (domain.Project, error) {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO projects (id, key, name, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		project.ID, project.Key, project.Name, project.Description, project.CreatedAt, project.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Project{}, fmt.Errorf("%w: project key %q", domain.ErrDuplicateKey, project.Key)
		}
		return domain.Project{}, fmt.Errorf("postgres: create project: %w", err)
	}
	return project, nil
}

// List returns all projects ordered by creation time.
func (r *ProjectRepository) List(ctx context.Context) ([]domain.Project, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, key, name, description, created_at, updated_at
		FROM projects
		ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list projects: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Project, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan project: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list projects: %w", err)
	}
	return out, nil
}

// GetByID returns the project with the given ID, or domain.ErrNotFound if
// it does not exist.
func (r *ProjectRepository) GetByID(ctx context.Context, id string) (domain.Project, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, key, name, description, created_at, updated_at
		FROM projects
		WHERE id = $1`, id)

	p, err := scanProject(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Project{}, fmt.Errorf("%w: project %q", domain.ErrNotFound, id)
		}
		return domain.Project{}, fmt.Errorf("postgres: get project: %w", err)
	}
	return p, nil
}

func scanProject(row rowScanner) (domain.Project, error) {
	var p domain.Project
	err := row.Scan(&p.ID, &p.Key, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}
