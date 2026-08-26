package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/SSujitX/redgres/internal/backup"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/go-chi/chi/v5"
)

const postgresDropCatalogMessage = "Backup catalog is not configured."

type postgresDropRequest struct {
	DatabaseConfirmation string `json:"database_confirmation"`
	OwnerPassword        string `json:"owner_password"`
}

type postgresDropResponse struct {
	Dropped     string `json:"dropped"`
	DroppedRole string `json:"dropped_role,omitempty"`
	RequestID   string `json:"request_id"`
}

type postgresClusterIdentity interface {
	SystemIdentifier(ctx context.Context) (string, error)
}

type postgresValidatedDrop interface {
	postgresClusterIdentity
	DropAfterValidation(ctx context.Context, database string, beforeDrop func(context.Context) error) (postgresadmin.DropResult, error)
}

type postgresDropGateError struct {
	status  int
	code    string
	message string
}

func (e postgresDropGateError) Error() string { return "PostgreSQL drop backup gate failed" }

func dropGateDenied(reason string) bool {
	switch reason {
	case "Backup manifest is invalid.",
		"Backup is older than 24 hours.",
		"Backup cluster identity does not match.",
		"Backup has no matching PostgreSQL artifact.",
		"Off-host copy is incomplete.",
		"Restore evidence is missing or stale.":
		return true
	default:
		return false
	}
}

func (s *Server) handlePostgresDatabaseDrop(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.FeaturePostgresDrop {
		s.writeError(w, r, http.StatusForbidden, CodeForbidden, postgresDropOffMessage)
		return
	}
	database, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	var body postgresDropRequest
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	if body.DatabaseConfirmation != database {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, postgresDropConfirmMessage, map[string]string{"database_confirmation": "invalid"})
		return
	}
	sess := sessionFrom(r)
	meta := map[string]any{"database": database}
	clientIP := requestClientIP(r)
	if err := auth.Reauthenticate(s.db, sess.Username, body.OwnerPassword, clientIP, time.Now().UTC()); err != nil {
		switch {
		case errors.Is(err, auth.ErrRateLimited):
			w.Header().Set("Retry-After", strconv.Itoa(int(auth.RateLimitRemaining(err).Seconds())+1))
			_ = s.audit.Record(sess.Username, "postgres.database.drop", database, "failure", requestID(r), clientIP, meta)
			s.writeError(w, r, http.StatusTooManyRequests, CodeRateLimited, reauthLimitedMessage)
			return
		case errors.Is(err, auth.ErrReauthRequired):
			_ = s.audit.Record(sess.Username, "postgres.database.drop", database, "failure", requestID(r), clientIP, meta)
			s.writeError(w, r, http.StatusForbidden, CodeReauthRequired, "Owner password is incorrect")
			return
		default:
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), postgresDropTimeout)
	defer cancel()
	if s.postgres == nil {
		_ = s.audit.Record(sess.Username, "postgres.database.drop", database, "failure", requestID(r), requestClientIP(r), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	dropper, ok := s.postgres.(postgresValidatedDrop)
	if !ok {
		_ = s.audit.Record(sess.Username, "postgres.database.drop", database, "failure", requestID(r), requestClientIP(r), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	result, err := dropper.DropAfterValidation(ctx, database, func(ctx context.Context) error {
		if s.cfg.BackupCatalogDir == "" {
			return postgresDropGateError{status: http.StatusServiceUnavailable, code: CodeDependencyUnavailable, message: postgresDropCatalogMessage}
		}
		manifest, err := backup.LoadCurrent(s.cfg.BackupCatalogDir)
		if err != nil {
			return postgresDropGateError{status: http.StatusServiceUnavailable, code: CodeDependencyUnavailable, message: postgresDropCatalogMessage}
		}
		sysID, err := dropper.SystemIdentifier(ctx)
		if err != nil {
			return err
		}
		gate := backup.EvaluateDropGate(backup.DropGateInput{
			Database:         database,
			SystemIdentifier: sysID,
			Now:              time.Now().UTC(),
			Manifest:         manifest,
		})
		if !gate.Allowed {
			if dropGateDenied(gate.Reason) {
				return postgresDropGateError{status: http.StatusForbidden, code: CodeForbidden, message: gate.Reason}
			}
			return postgresDropGateError{status: http.StatusServiceUnavailable, code: CodeDependencyUnavailable, message: postgresDropCatalogMessage}
		}
		if err := backup.VerifyPostgresDatabaseArtifact(ctx, s.cfg.BackupCatalogDir, manifest, database); err != nil {
			return postgresDropGateError{status: http.StatusServiceUnavailable, code: CodeDependencyUnavailable, message: postgresDropCatalogMessage}
		}
		return nil
	})
	if err != nil {
		_ = s.audit.Record(sess.Username, "postgres.database.drop", database, "failure", requestID(r), requestClientIP(r), meta)
		var gateErr postgresDropGateError
		if errors.As(err, &gateErr) {
			s.writeError(w, r, gateErr.status, gateErr.code, gateErr.message)
			return
		}
		s.writePostgresError(w, r, err)
		return
	}
	successMeta := map[string]any{"database": result.Dropped, "owner": result.Owner}
	if result.DroppedRole != "" {
		successMeta["dropped_role"] = result.DroppedRole
	}
	if err := s.audit.Record(sess.Username, "postgres.database.drop", database, "success", requestID(r), requestClientIP(r), successMeta); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresDropResponse{
		Dropped:     result.Dropped,
		DroppedRole: result.DroppedRole,
		RequestID:   requestID(r),
	})
}
