package postgres

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// dsnEnvVar names the environment variable that points integration tests at
// a real PostgreSQL instance. Tests skip cleanly when it's unset, so
// `go test ./...` stays fast and DB-free by default.
const dsnEnvVar = "FLAGSHIP_TEST_DATABASE_URL"

var (
	migrateOnce sync.Once
	migrateErr  error
)

// testPool returns a connection pool to the test database, with all tables
// truncated for isolation, skipping the test if dsnEnvVar isn't set.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv(dsnEnvVar)
	if dsn == "" {
		t.Skipf("%s not set; skipping PostgreSQL integration test", dsnEnvVar)
	}

	// The schema is migrated once per test binary, but the result is
	// recorded rather than failing only the test that happened to run
	// first -- otherwise every later test would run against a schema that
	// was never created and fail for confusing, unrelated reasons.
	migrateOnce.Do(func() { migrateErr = Migrate(dsn) })
	if migrateErr != nil {
		t.Fatalf("Migrate() unexpected error: %v", migrateErr)
	}

	pool, err := Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Connect() unexpected error: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(context.Background(), `TRUNCATE TABLE projects, environments, flags, config_events CASCADE`); err != nil {
		t.Fatalf("truncating test tables: %v", err)
	}

	return pool
}

// seedEnvironment inserts a project and an environment with the given ID
// directly, so flag-repository tests have a valid parent row to satisfy the
// flags table's foreign key (config_version starts at 0, matching a fresh
// environment).
func seedEnvironment(t *testing.T, pool *pgxpool.Pool, environmentID string) {
	t.Helper()
	ctx := context.Background()

	projectID := environmentID + "-project"
	_, err := pool.Exec(ctx, `
		INSERT INTO projects (id, key, name, description, created_at, updated_at)
		VALUES ($1, $1, $1, '', now(), now())`, projectID)
	if err != nil {
		t.Fatalf("seeding project: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO environments (id, project_id, key, name, created_at, updated_at)
		VALUES ($1, $2, $1, $1, now(), now())`, environmentID, projectID)
	if err != nil {
		t.Fatalf("seeding environment: %v", err)
	}
}
