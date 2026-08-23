package httpapi

import (
	"errors"
	"net/http"
	"net/url"

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
