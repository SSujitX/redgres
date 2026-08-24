package httpapi

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/SSujitX/redgres/internal/audit"
)

const (
	// auditCursorPrefix versions the opaque cursor payload so the ordering key
	// can change later without breaking the published contract.
	auditCursorPrefix = "a1:"
	// maxAuditCursorLen bounds the encoded input before decoding so a hostile
	// query string cannot force a large allocation.
	maxAuditCursorLen = 64
)

type auditEventsBody struct {
	Events     []audit.EventSummary `json:"events"`
	HasMore    bool                 `json:"has_more"`
	NextCursor string               `json:"next_cursor,omitempty"`
	Limit      int                  `json:"limit"`
	RequestID  string               `json:"request_id"`
}

func (s *Server) handleAuditEvents(w http.ResponseWriter, r *http.Request) {
	before, ok := decodeAuditCursor(r.URL.Query().Get("cursor"))
	if !ok {
		// The submitted value is attacker-controlled, so it is never echoed.
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid cursor", map[string]string{"cursor": "invalid"})
		return
	}
	requested, ok := parseOptionalInt(r.URL.Query().Get("limit"), 0)
	if !ok {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid limit", map[string]string{"limit": "invalid"})
		return
	}
	limit := audit.ClampListLimit(requested)

	page, err := s.audit.List(r.Context(), before, limit)
	if err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}

	body := auditEventsBody{
		Events:    page.Events,
		HasMore:   page.HasMore,
		Limit:     limit,
		RequestID: requestID(r),
	}
	if page.HasMore && len(page.Events) > 0 {
		body.NextCursor = encodeAuditCursor(page.Events[len(page.Events)-1].ID)
	}
	s.writeJSON(w, r, http.StatusOK, body)
}

func encodeAuditCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(auditCursorPrefix + strconv.FormatInt(id, 10)))
}

// decodeAuditCursor converts an opaque cursor into an exclusive upper bound on
// the event id. An empty cursor selects the newest page. A well-formed cursor
// that no longer matches a row is valid and simply yields older events, so only
// encoding and range failures are rejected.
func decodeAuditCursor(raw string) (int64, bool) {
	if raw == "" {
		return 0, true
	}
	if len(raw) > maxAuditCursorLen {
		return 0, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, false
	}
	digits, ok := strings.CutPrefix(string(decoded), auditCursorPrefix)
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	// Require canonical digits so "+5", "007", and "-0" are rejected rather
	// than silently aliasing a valid cursor.
	if strconv.FormatInt(id, 10) != digits {
		return 0, false
	}
	return id, true
}
