package httpapi

import (
	"context"
	"net/http"
	"time"
)

func live(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func readiness(check func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if check == nil || check(ctx) != nil {
			writeErrorResponse(w, http.StatusServiceUnavailable, "not_ready", "The server is not ready.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}
