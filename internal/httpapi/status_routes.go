package httpapi

import (
	"context"
	"net/http"

	"github.com/SSujitX/redgres/internal/platform"
	"github.com/SSujitX/redgres/internal/postgresadmin"
)

type statusBody struct {
	Components []platform.Component `json:"components"`
	RequestID  string               `json:"request_id"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var postgresPing platform.PingFunc
	if s.postgres != nil {
		postgresPing = s.postgres.Ping
	}
	components := platform.Collect(r.Context(), s.pingState, postgresPing)
	s.writeJSON(w, r, http.StatusOK, statusBody{Components: components, RequestID: requestID(r)})
}

func (s *Server) pingState(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return postgresadmin.ErrUnavailable
	}
	return nil
}
