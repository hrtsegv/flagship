package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/hrtsegv/flagship/internal/app"
	"github.com/hrtsegv/flagship/internal/domain"
)

// streamHeartbeatInterval is comfortably longer than any request this
// server should ever take to complete, so it never fires during normal
// use or tests -- its only job is to keep the connection alive through
// intermediaries (proxies, load balancers) that time out idle streams.
const streamHeartbeatInterval = 20 * time.Second

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

func (h *flagHandler) delete(w http.ResponseWriter, r *http.Request) {
	flagID := r.PathValue("flagID")

	if err := h.svc.Delete(r.Context(), flagID); err != nil {
		writeError(w, err)
		return
	}

	writeNoContent(w)
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

// stream serves a live, resumable feed of an environment's configuration
// events as Server-Sent Events. A client resuming after a disconnect
// sends its last-applied version as Last-Event-ID; the response replays
// whatever it missed (if the server can) before switching to live
// delivery, with no gap or duplicate at that handoff (see
// FlagService.Stream / FlagRepository.SubscribeFrom). A client that has
// fallen behind further than the server retains, or that provides no
// Last-Event-ID, just starts receiving live events with no replay: the
// SDK's own version-gap detection covers the rest.
func (h *flagHandler) stream(w http.ResponseWriter, r *http.Request) {
	environmentID := r.PathValue("environmentID")

	sinceVersion := 0
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeErrorResponse(w, http.StatusBadRequest, "invalid_last_event_id", "Last-Event-ID must be a non-negative integer.")
			return
		}
		sinceVersion = parsed
	}

	sub, err := h.svc.Stream(r.Context(), environmentID, sinceVersion)
	if err != nil {
		writeError(w, err)
		return
	}
	defer sub.Unsubscribe()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErrorResponse(w, http.StatusInternalServerError, "internal_error", "Streaming is not supported.")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for _, e := range sub.Events {
		if !writeConfigEvent(w, sub.Environment.Key, e) {
			return
		}
	}
	flusher.Flush()

	heartbeat := time.NewTicker(streamHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-sub.Live:
			if !ok {
				return
			}
			if !writeConfigEvent(w, sub.Environment.Key, e) {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeConfigEvent writes one SSE frame for e and reports whether the
// write succeeded. The event ID is the configuration version itself (see
// ADR-0004) -- no separate ID is needed, and it's what a reconnecting
// client echoes back as Last-Event-ID to resume.
func writeConfigEvent(w http.ResponseWriter, environmentKey string, e domain.ConfigEvent) bool {
	data, err := json.Marshal(toConfigEventPayload(environmentKey, e))
	if err != nil {
		return false
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: configuration.updated\ndata: %s\n\n", e.Version, data)
	return err == nil
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
