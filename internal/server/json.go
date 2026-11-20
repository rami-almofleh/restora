package server

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// ErrorBody is the single error format of the REST API.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail describes what happened (Message) and what the user can do (Hint).
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes an error in the common API error format.
func WriteError(w http.ResponseWriter, status int, code, message, hint string) {
	WriteJSON(w, status, ErrorBody{Error: ErrorDetail{Code: code, Message: message, Hint: hint}})
}

// NotFound answers unknown API paths with a JSON error instead of the web UI.
func NotFound(w http.ResponseWriter, _ *http.Request) {
	WriteError(w, http.StatusNotFound, "not_found", "This API endpoint does not exist.", "Check the path and the API version.")
}

// ParseLimit reads the "limit" query parameter of a list endpoint. Zero means "not given". When the value is
// invalid it answers with the common error and returns false.
func ParseLimit(w http.ResponseWriter, r *http.Request) (limit int, ok bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		WriteError(w, http.StatusBadRequest, "invalid_limit",
			"The limit must be a positive number.", "Use a value between 1 and 200.")
		return 0, false
	}
	return n, true
}
