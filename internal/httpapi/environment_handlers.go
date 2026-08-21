package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/hrtsegv/flagship/internal/app"
)

type environmentHandler struct {
	svc *app.EnvironmentService
}

func (h *environmentHandler) create(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")

	var req createEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "invalid_request_body", "The request body is not valid JSON.")
		return
	}

	env, err := h.svc.Create(r.Context(), projectID, app.CreateEnvironmentInput{
		Key:  req.Key,
		Name: req.Name,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toEnvironmentResponse(env))
}

func (h *environmentHandler) list(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")

	envs, err := h.svc.ListByProject(r.Context(), projectID)
	if err != nil {
		writeError(w, err)
		return
	}

	resp := make([]environmentResponse, 0, len(envs))
	for _, e := range envs {
		resp = append(resp, toEnvironmentResponse(e))
	}
	writeJSON(w, http.StatusOK, resp)
}
