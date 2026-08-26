package httpapi

import (
	"context"
	"net/http"

	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/go-chi/chi/v5"
)

type postgresDuplicateRequest struct {
	Database string `json:"database"`
	Owner    string `json:"owner"`
}

func (s *Server) handlePostgresDatabasesDuplicate(w http.ResponseWriter, r *http.Request) {
	source, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	var body postgresDuplicateRequest
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	fields := map[string]string{}
	if err := postgresadmin.ValidateIdentifier(body.Database); err != nil {
		fields["database"] = "invalid"
	}
	if err := postgresadmin.ValidateIdentifier(body.Owner); err != nil {
		fields["owner"] = "invalid"
	}
	if len(fields) > 0 {
		msg := "Invalid database name"
		if _, ok := fields["database"]; !ok {
			msg = "Invalid role name."
		}
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, msg, fields)
		return
	}
	if source == body.Database {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, postgresDuplicateSameNameMessage, map[string]string{"database": "invalid"})
		return
	}
	policy := postgresadmin.NewPolicy(s.cfg)
	if policy.DatabaseDenied(body.Database) || policy.OwnerDenied(body.Owner) {
		s.writeError(w, r, http.StatusForbidden, CodeProtectedResource, "This PostgreSQL name is protected")
		return
	}
	sess := sessionFrom(r)
	meta := map[string]any{"database": body.Database, "owner": body.Owner, "source": source}
	if s.postgres == nil {
		_ = s.audit.Record(sess.Username, "postgres.database.duplicate", body.Database, "failure", requestID(r), requestClientIP(r), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), postgresDuplicateTimeout)
	defer cancel()
	created, err := s.postgres.Duplicate(ctx, source, body.Database, body.Owner)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	cred := postgresRevealCredential{
		Username: created.Owner,
		Password: created.Password,
		OneTime:  false,
	}
	var urls postgresRevealURLs
	if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresDirectPort != "" {
		if u, urlErr := postgresadmin.ProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresDirectPort, created.Owner, created.Password, created.Database); urlErr == nil {
			urls.Direct = u
		}
	}
	if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresPooledPort != "" {
		if u, urlErr := postgresadmin.ProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresPooledPort, created.Owner, created.Password, created.Database); urlErr == nil {
			urls.Pooled = u
		}
	}
	if urls.Direct != "" || urls.Pooled != "" {
		cred.URLs = &urls
	}
	if err := s.audit.Record(sess.Username, "postgres.database.duplicate", created.Database, "success", requestID(r), requestClientIP(r), map[string]any{"database": created.Database, "owner": created.Owner, "source": source}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	s.writeJSON(w, r, http.StatusCreated, postgresRevealResponse{
		Resource:   postgresRevealResource{Type: "postgres_database", Name: created.Database},
		Credential: cred,
		RequestID:  requestID(r),
	})
}
