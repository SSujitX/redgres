package httpapi

import (
	"net/http"
)

type healthzBody struct {
	Status    string `json:"status"`
	RequestID string `json:"request_id"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "Control-plane storage is unavailable")
		return
	}
	s.writeJSON(w, r, http.StatusOK, healthzBody{Status: "ok", RequestID: requestID(r)})
}
