// Package respond is the JSON error envelope every hand-written response
// uses: {"code": "...", "message": "...", "details"?: {...}}. Its own package
// so future middleware can use it without importing internal/api.
package respond

import (
	"encoding/json"
	"net/http"
)

// Envelope is the error body shape.
type Envelope struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// JSON encodes v with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes the standard envelope.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, Envelope{Code: code, Message: message})
}
