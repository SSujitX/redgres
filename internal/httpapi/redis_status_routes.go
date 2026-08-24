package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/SSujitX/redgres/internal/redisadmin"
)

const redisStatusTimeout = 2 * time.Second

type redisStatusBody struct {
	State     string              `json:"state"`
	Reason    string              `json:"reason,omitempty"`
	Metrics   *redisadmin.Metrics `json:"metrics,omitempty"`
	RequestID string              `json:"request_id"`
}

func (s *Server) handleRedisStatus(w http.ResponseWriter, r *http.Request) {
	if s.redis == nil {
		s.writeJSON(w, r, http.StatusOK, redisStatusBody{State: "not_configured", RequestID: requestID(r)})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), redisStatusTimeout)
	defer cancel()
	metrics, err := s.redis.Status(ctx)
	if errors.Is(err, redisadmin.ErrNotConfigured) {
		s.writeJSON(w, r, http.StatusOK, redisStatusBody{State: "not_configured", RequestID: requestID(r)})
		return
	}
	if err != nil {
		reason := "unreachable"
		if errors.Is(err, redisadmin.ErrAuthFailed) {
			reason = "auth_failed"
		} else if errors.Is(err, redisadmin.ErrPermissionDenied) {
			reason = "permission_denied"
		}
		s.writeJSON(w, r, http.StatusOK, redisStatusBody{
			State:     "unavailable",
			Reason:    reason,
			RequestID: requestID(r),
		})
		return
	}
	s.writeJSON(w, r, http.StatusOK, redisStatusBody{
		State:     "ok",
		Metrics:   &metrics,
		RequestID: requestID(r),
	})
}
