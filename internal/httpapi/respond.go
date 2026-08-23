// Package httpapi is the HTTP transport layer. It decodes requests into
// application-layer inputs, calls the application layer, and encodes
// responses/errors as JSON. It contains no domain or persistence logic.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hrtsegv/flagship/internal/domain"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func writeNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeErrorResponse(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}

// writeError maps a domain/application error to an HTTP status code and the
// documented JSON error format, without exposing internal error details.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeErrorResponse(w, http.StatusNotFound, "resource_not_found", "The requested resource was not found.")
	case errors.Is(err, domain.ErrDuplicateKey):
		writeErrorResponse(w, http.StatusConflict, "duplicate_key", "A resource with this key already exists.")
	case errors.Is(err, domain.ErrValidation):
		writeErrorResponse(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		writeErrorResponse(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
