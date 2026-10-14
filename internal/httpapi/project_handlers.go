package httpapi

import (
	"net/http"

	"github.com/hrtsegv/flagship/internal/app"
)

type projectHandler struct {
	svc *app.ProjectService
}

func (h *projectHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if !decodeRequest(w, r, &req) {
		return
	}

	project, err := h.svc.Create(r.Context(), app.CreateProjectInput{
		Key:         req.Key,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toProjectResponse(project))
}

func (h *projectHandler) list(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	resp := make([]projectResponse, 0, len(projects))
	for _, p := range projects {
		resp = append(resp, toProjectResponse(p))
	}
	writeJSON(w, http.StatusOK, resp)
}
