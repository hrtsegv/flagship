package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/hrtsegv/flagship/internal/app"
	"github.com/hrtsegv/flagship/internal/domain"
)

type flagHandler struct {
	svc *app.FlagService
}

func (h *flagHandler) create(w http.ResponseWriter, r *http.Request) {
	environmentID := r.PathValue("environmentID")

	var req createFlagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "invalid_request_body", "The request body is not valid JSON.")
		return
	}

	flag, err := h.svc.Create(r.Context(), environmentID, app.CreateFlagInput{
		Key:         req.Key,
		Name:        req.Name,
		Description: req.Description,
		Enabled:     req.Enabled,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toFlagResponse(flag))
}

func (h *flagHandler) list(w http.ResponseWriter, r *http.Request) {
	environmentID := r.PathValue("environmentID")

	flags, err := h.svc.ListByEnvironment(r.Context(), environmentID)
	if err != nil {
		writeError(w, err)
		return
	}

	resp := make([]flagResponse, 0, len(flags))
	for _, f := range flags {
		resp = append(resp, toFlagResponse(f))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *flagHandler) get(w http.ResponseWriter, r *http.Request) {
	flagID := r.PathValue("flagID")

	flag, err := h.svc.Get(r.Context(), flagID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toFlagResponse(flag))
}

func (h *flagHandler) configuration(w http.ResponseWriter, r *http.Request) {
	environmentID := r.PathValue("environmentID")

	env, version, flags, err := h.svc.Snapshot(r.Context(), environmentID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toConfigurationResponse(env, version, flags))
}

func (h *flagHandler) update(w http.ResponseWriter, r *http.Request) {
	flagID := r.PathValue("flagID")

	var req updateFlagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "invalid_request_body", "The request body is not valid JSON.")
		return
	}

	flag, err := h.svc.Update(r.Context(), flagID, domain.FlagPatch{
		Name:        req.Name,
		Description: req.Description,
		Enabled:     req.Enabled,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toFlagResponse(flag))
}
