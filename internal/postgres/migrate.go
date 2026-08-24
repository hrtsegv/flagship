package postgres

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies every pending schema migration to the database at dsn,
// creating the schema if it doesn't exist yet. It's safe to call on every
// process startup: already-applied migrations are skipped.
func Migrate(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("postgres: open for migration: %w", err)
	}
	defer db.Close()

	source, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("postgres: load embedded migrations: %w", err)
	}

	target, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("postgres: init migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", target)
	if err != nil {
		return fmt.Errorf("postgres: init migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("postgres: apply migrations: %w", err)
	}
	return nil
}
