package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SSujitX/redgres/internal/platform"
	"github.com/SSujitX/redgres/internal/postgresadmin"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 50
	searchTimeout      = 2 * time.Second
)

type searchResponse struct {
	Groups    []platform.SearchGroup `json:"groups"`
	Limit     int                    `json:"limit"`
	RequestID string                 `json:"request_id"`
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	runes := utf8.RuneCountInString(q)
	if runes < 1 {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Query is too short", map[string]string{"q": "too_short"})
		return
	}
	if runes > postgresadmin.MaxRowQueryRunes {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Query is too long", map[string]string{"q": "too_long"})
		return
	}

	requested, ok := parseOptionalInt(r.URL.Query().Get("limit"), 0)
	if !ok {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid limit", map[string]string{"limit": "invalid"})
		return
	}
	limit := clampSearchLimit(requested)

	s.writeJSON(w, r, http.StatusOK, searchResponse{
		Groups:    s.searchGroups(r.Context(), q, limit),
		Limit:     limit,
		RequestID: requestID(r),
	})
}

func (s *Server) searchGroups(ctx context.Context, q string, limit int) []platform.SearchGroup {
	if s.postgres == nil {
		return platform.ResourceGroups(platform.PostgresSearch{Status: "not_configured"})
	}
	searchCtx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	result, err := s.postgres.Search(searchCtx, q, limit)
	if err != nil {
		status := "unavailable"
		if errors.Is(err, postgresadmin.ErrNotConfigured) {
			status = "not_configured"
		}
		return platform.ResourceGroups(platform.PostgresSearch{Status: status})
	}
	names := make([]string, 0, len(result.Hits))
	for _, hit := range result.Hits {
		names = append(names, hit.Name)
	}
	return platform.ResourceGroups(platform.PostgresSearch{
		Status:    "ok",
		Truncated: result.Truncated,
		Names:     names,
	})
}

func clampSearchLimit(n int) int {
	if n <= 0 || n > maxSearchLimit {
		return defaultSearchLimit
	}
	return n
}
