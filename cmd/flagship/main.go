// Command flagship runs the Flagship control-plane HTTP API backed by an
// in-memory repository.
package main

import (
	"log"
	"net/http"

	"github.com/hrtsegv/flagship/internal/app"
	"github.com/hrtsegv/flagship/internal/httpapi"
	"github.com/hrtsegv/flagship/internal/memory"
)

const addr = ":8080"

func main() {
	projectRepo := memory.NewProjectRepository()
	environmentRepo := memory.NewEnvironmentRepository()
	flagRepo := memory.NewFlagRepository()

	projectSvc := app.NewProjectService(projectRepo)
	environmentSvc := app.NewEnvironmentService(environmentRepo, projectRepo)
	flagSvc := app.NewFlagService(flagRepo, environmentRepo)

	handler := httpapi.NewServer(projectSvc, environmentSvc, flagSvc)

	log.Printf("flagship server listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
