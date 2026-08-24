// Package postgres provides a PostgreSQL-backed implementation of the
// repository interfaces used by the application layer, as an alternative to
// internal/memory for production deployments.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// rowScanner is satisfied by both pgx.Row (QueryRow) and pgx.Rows (Query),
// so a single scan helper per table can serve both a single-row lookup and
// a multi-row list.
type rowScanner interface {
	Scan(dest ...any) error
}

// Connect opens a connection pool to the database at dsn. Callers should
// Close the pool when done (typically for the lifetime of the process).
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return pool, nil
}
