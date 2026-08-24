// Command flagship runs the Flagship control-plane HTTP API backed by
// PostgreSQL.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/hrtsegv/flagship/internal/app"
	"github.com/hrtsegv/flagship/internal/httpapi"
	"github.com/hrtsegv/flagship/internal/postgres"
)

const addr = ":8080"

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	if err := postgres.Migrate(dsn); err != nil {
		log.Fatalf("running migrations: %v", err)
	}

	pool, err := postgres.Connect(context.Background(), dsn)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer pool.Close()

	projectRepo := postgres.NewProjectRepository(pool)
	environmentRepo := postgres.NewEnvironmentRepository(pool)
	flagRepo := postgres.NewFlagRepository(pool)

	projectSvc := app.NewProjectService(projectRepo)
	environmentSvc := app.NewEnvironmentService(environmentRepo, projectRepo)
	flagSvc := app.NewFlagService(flagRepo, environmentRepo)

	handler := httpapi.NewServer(projectSvc, environmentSvc, flagSvc)

	log.Printf("flagship server listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
