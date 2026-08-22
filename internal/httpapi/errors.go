package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

const (
	CodeUnauthorized          = "unauthorized"
	CodeForbidden             = "forbidden"
	CodeCSRFInvalid           = "csrf_invalid"
	CodeRateLimited           = "rate_limited"
	CodeValidationError       = "validation_error"
	CodeProtectedResource     = "protected_resource"
	CodeConflict              = "conflict"
	CodeNotFound              = "not_found"
	CodeMethodNotAllowed      = "method_not_allowed"
	CodeReauthRequired        = "reauth_required"
	CodeDependencyUnavailable = "dependency_unavailable"
	CodeOperationInProgress   = "operation_in_progress"
	CodeInternal              = "internal"
)

type apiError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type errorBody struct {
	Error     apiError `json:"error"`
	RequestID string   `json:"request_id"`
}

func (s *Server) writeJSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	s.writeJSON(w, r, status, errorBody{
		Error:     apiError{Code: code, Message: message},
		RequestID: requestID(r),
	})
}

func isAPIPath(path string) bool {
	path = strings.TrimRight(path, "/")
	return path == "/api" || strings.HasPrefix(path, "/api/")
}
