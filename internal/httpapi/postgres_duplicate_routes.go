package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/SSujitX/redgres/internal/operations"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/go-chi/chi/v5"
)

type postgresDuplicateRequest struct {
	Database string `json:"database"`
	Owner    string `json:"owner"`
}

type postgresDuplicateAcceptedBody struct {
	Operation postgresDuplicateAcceptedOperation `json:"operation"`
	RequestID string                             `json:"request_id"`
}

type postgresDuplicateAcceptedOperation struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type duplicatePreparer interface {
	PrepareDuplicate(ctx context.Context, source, database, owner string) error
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
	if s.operations == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	preparer, ok := s.postgres.(duplicatePreparer)
	if !ok {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), postgresDuplicateTimeout)
	defer cancel()
	if err := preparer.PrepareDuplicate(ctx, source, body.Database, body.Owner); err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	id, err := operations.NewID()
	if err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	op := operations.Operation{
		ID:                id,
		Action:            operations.ActionDuplicate,
		Actor:             sess.Username,
		Target:            body.Database,
		AcceptedRequestID: requestID(r),
		Result: &operations.DuplicateResult{
			Database: body.Database,
			Owner:    body.Owner,
			Source:   source,
		},
	}
	locks := []operations.ResourceLock{
		{Kind: operations.ResourceDatabase, Name: source},
		{Kind: operations.ResourceDatabase, Name: body.Database},
		{Kind: operations.ResourceRole, Name: body.Owner},
	}
	if err := s.operations.InsertQueued(ctx, op, locks); err != nil {
		if errors.Is(err, operations.ErrLockHeld) {
			s.writeError(w, r, http.StatusConflict, CodeOperationInProgress, postgresDuplicateInProgressMessage)
			return
		}
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusAccepted, postgresDuplicateAcceptedBody{
		Operation: postgresDuplicateAcceptedOperation{ID: id, Status: string(operations.StatusQueued)},
		RequestID: requestID(r),
	})
}
