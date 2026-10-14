package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maxRequestBodyBytes = 1 << 20

// decodeRequest accepts one JSON object with known fields. Decode fully
// before calling a use case so invalid input cannot partially mutate state.
func decodeRequest[T any](w http.ResponseWriter, r *http.Request, target *T) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()
	var decoded *T
	err := decoder.Decode(&decoded)
	if err == nil && decoded == nil {
		err = errors.New("expected a JSON object")
	}
	if err == nil {
		var extra json.RawMessage
		err = decoder.Decode(&extra)
		if err == io.EOF {
			*target = *decoded
			return true
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeErrorResponse(w, http.StatusRequestEntityTooLarge, "request_body_too_large", "The request body exceeds the 1 MiB limit.")
	} else {
		writeErrorResponse(w, http.StatusBadRequest, "invalid_request_body", "The request body must be one JSON object containing only supported fields.")
	}
	return false
}
