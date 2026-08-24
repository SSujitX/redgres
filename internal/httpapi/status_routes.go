package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/SSujitX/redgres/internal/platform"
	"github.com/SSujitX/redgres/internal/postgresadmin"
)

type statusBody struct {
	Components []platform.Component `json:"components"`
	RequestID  string               `json:"request_id"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	components := platform.Collect(r.Context(), s.pingState, s.postgresPing)
	s.writeJSON(w, r, http.StatusOK, statusBody{Components: components, RequestID: requestID(r)})
}

func (s *Server) pingState(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return postgresadmin.ErrUnavailable
	}
	return nil
}

func (s *Server) postgresPing(ctx context.Context) error {
	if s.postgres == nil {
		return platform.ErrNotConfigured
	}
	err := s.postgres.Ping(ctx)
	if errors.Is(err, postgresadmin.ErrNotConfigured) {
		return platform.ErrNotConfigured
	}
	return err
}
