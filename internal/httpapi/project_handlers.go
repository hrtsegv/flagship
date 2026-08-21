package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/hrtsegv/flagship/internal/app"
)

type projectHandler struct {
	svc *app.ProjectService
}

func (h *projectHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "invalid_request_body", "The request body is not valid JSON.")
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
