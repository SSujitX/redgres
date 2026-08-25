package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"unicode/utf8"

	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/go-chi/chi/v5"
)

type postgresListBody struct {
	postgresadmin.ListResult
	RequestID string `json:"request_id"`
}

type postgresDetailsBody struct {
	Database  postgresadmin.DatabaseDetails `json:"database"`
	RequestID string                        `json:"request_id"`
}

func (s *Server) handlePostgresDatabases(w http.ResponseWriter, r *http.Request) {
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	result, err := s.postgres.List(r.Context())
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresListBody{ListResult: result, RequestID: requestID(r)})
}

func (s *Server) handlePostgresDatabase(w http.ResponseWriter, r *http.Request) {
	name, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	details, err := s.postgres.Details(r.Context(), name)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresDetailsBody{Database: details, RequestID: requestID(r)})
}

type postgresConnectionBody struct {
	postgresadmin.Connection
	MaskedDirectURL string `json:"masked_direct_url,omitempty"`
	MaskedPooledURL string `json:"masked_pooled_url,omitempty"`
	RequestID       string `json:"request_id"`
}

func (s *Server) handlePostgresConnection(w http.ResponseWriter, r *http.Request) {
	name, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	conn, err := s.postgres.Connection(r.Context(), name)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	body := postgresConnectionBody{Connection: conn, RequestID: requestID(r)}
	if conn.SavedCredential.Status == "present" {
		if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresDirectPort != "" {
			if u, urlErr := postgresadmin.MaskedProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresDirectPort, conn.Owner, conn.Database); urlErr == nil {
				body.MaskedDirectURL = u
			}
		}
		if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresPooledPort != "" {
			if u, urlErr := postgresadmin.MaskedProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresPooledPort, conn.Owner, conn.Database); urlErr == nil {
				body.MaskedPooledURL = u
			}
		}
	}
	s.writeJSON(w, r, http.StatusOK, body)
}

type postgresTablesBody struct {
	postgresadmin.TableListResult
	RequestID string `json:"request_id"`
}

func (s *Server) handlePostgresTables(w http.ResponseWriter, r *http.Request) {
	name, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	result, err := s.postgres.Tables(r.Context(), name)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresTablesBody{TableListResult: result, RequestID: requestID(r)})
}

type postgresRowsBody struct {
	postgresadmin.RowPage
	RequestID string `json:"request_id"`
}

type postgresSecurityBody struct {
	postgresadmin.SecurityOverview
	RequestID string `json:"request_id"`
}

func (s *Server) handlePostgresRows(w http.ResponseWriter, r *http.Request) {
	database, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	schema, err := decodePathIdentifier(chi.URLParam(r, "schema"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid schema name")
		return
	}
	table, err := decodePathIdentifier(chi.URLParam(r, "table"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid table name")
		return
	}
	q := r.URL.Query().Get("q")
	if utf8.RuneCountInString(q) > postgresadmin.MaxRowQueryRunes {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Query is too long", map[string]string{"q": "too_long"})
		return
	}
	offset, ok := parseOptionalInt(r.URL.Query().Get("offset"), 0)
	if !ok {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid offset", map[string]string{"offset": "invalid"})
		return
	}
	limit, ok := parseOptionalInt(r.URL.Query().Get("limit"), 0)
	if !ok {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid limit", map[string]string{"limit": "invalid"})
		return
	}
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	result, err := s.postgres.Rows(r.Context(), database, schema, table, q, offset, limit)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresRowsBody{RowPage: result, RequestID: requestID(r)})
}

func (s *Server) handlePostgresSecurity(w http.ResponseWriter, r *http.Request) {
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	result, err := s.postgres.SecurityOverview(r.Context())
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresSecurityBody{SecurityOverview: result, RequestID: requestID(r)})
}

func parseOptionalInt(raw string, fallback int) (int, bool) {
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
}

func decodePathIdentifier(raw string) (string, error) {
	name, err := url.PathUnescape(raw)
	if err != nil {
		return "", postgresadmin.ErrInvalidIdentifier
	}
	if err := postgresadmin.ValidateIdentifier(name); err != nil {
		return "", err
	}
	return name, nil
}

func (s *Server) writePostgresError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, postgresadmin.ErrInvalidIdentifier):
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
	case errors.Is(err, postgresadmin.ErrNotFound):
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Not found")
	case errors.Is(err, postgresadmin.ErrUnavailable):
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
	default:
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
	}
}
