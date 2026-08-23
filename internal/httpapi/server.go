package httpapi

import (
	"net/http"

	"github.com/hrtsegv/flagship/internal/app"
)

// NewServer wires the HTTP routes to their handlers and returns the
// resulting http.Handler.
func NewServer(projects *app.ProjectService, environments *app.EnvironmentService, flags *app.FlagService) http.Handler {
	mux := http.NewServeMux()

	ph := &projectHandler{svc: projects}
	eh := &environmentHandler{svc: environments}
	fh := &flagHandler{svc: flags}

	mux.HandleFunc("POST /api/v1/projects", ph.create)
	mux.HandleFunc("GET /api/v1/projects", ph.list)

	mux.HandleFunc("POST /api/v1/projects/{projectID}/environments", eh.create)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/environments", eh.list)

	mux.HandleFunc("POST /api/v1/environments/{environmentID}/flags", fh.create)
	mux.HandleFunc("GET /api/v1/environments/{environmentID}/flags", fh.list)
	mux.HandleFunc("GET /api/v1/environments/{environmentID}/configuration", fh.configuration)
	mux.HandleFunc("GET /api/v1/environments/{environmentID}/stream", fh.stream)

	mux.HandleFunc("GET /api/v1/flags/{flagID}", fh.get)
	mux.HandleFunc("PATCH /api/v1/flags/{flagID}", fh.update)
	mux.HandleFunc("DELETE /api/v1/flags/{flagID}", fh.delete)

	return mux
}
