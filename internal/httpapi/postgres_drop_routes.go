package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/go-chi/chi/v5"
)

type postgresDropRequest struct {
	DatabaseConfirmation string `json:"database_confirmation"`
	OwnerPassword        string `json:"owner_password"`
}

type postgresDropResponse struct {
	Dropped     string `json:"dropped"`
	DroppedRole string `json:"dropped_role,omitempty"`
	RequestID   string `json:"request_id"`
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
	result, err := s.postgres.Drop(ctx, database)
	if err != nil {
		_ = s.audit.Record(sess.Username, "postgres.database.drop", database, "failure", requestID(r), requestClientIP(r), meta)
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
