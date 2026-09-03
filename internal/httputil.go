// Package httputil provides small shared helpers so every handler responds
// with consistent JSON and predictable HTTP status codes.
package httputil

import (
	"encoding/json"
	"log"
	"net/http"
)

// JSON writes v as a JSON response body with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("httputil: encoding response: %v", err)
	}
}

// Error writes a JSON error body: {"error": "message"}.
func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}

// ServerError logs the underlying error and writes a generic 500 response,
// so internal details are never leaked to the client.
func ServerError(w http.ResponseWriter, err error) {
	log.Printf("internal server error: %v", err)
	Error(w, http.StatusInternalServerError, "internal server error")
}

// DecodeJSON reads and decodes a JSON request body into v, rejecting
// unknown fields so typos in client payloads surface as errors rather than
// being silently ignored.
func DecodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
