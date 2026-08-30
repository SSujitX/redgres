package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) serializeDomainMutation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		disconnecting := r.Method == http.MethodDelete && r.URL.Path == "/api/v1/domain"
		if !disconnecting && s.db != nil {
			if dep, err := (domainStore{s.db}).Get(r.Context()); err == nil && dep.DisconnectPending {
				s.writeError(w, r, http.StatusConflict, CodeOperationInProgress, "Domain disconnect cleanup is pending")
				return
			}
		}
		if !disconnecting && s.cfg.TLSIssueRequestFile != "" {
			if info, err := os.Lstat(s.cfg.TLSIssueRequestFile); err == nil && info.Mode().IsRegular() {
				s.writeError(w, r, http.StatusConflict, CodeOperationInProgress, "A domain operation is already in progress")
				return
			}
		}
		if !disconnecting && s.cfg.TLSIssueResultFile != "" {
			active := filepath.Join(filepath.Dir(s.cfg.TLSIssueResultFile), "active.request")
			if info, err := os.Lstat(active); err == nil && info.Mode().IsRegular() {
				s.writeError(w, r, http.StatusConflict, CodeOperationInProgress, "A domain operation is already in progress")
				return
			}
		}
		s.domainMutationMu.Lock()
		if s.domainMutationActive {
			s.domainMutationMu.Unlock()
			s.writeError(w, r, http.StatusConflict, CodeOperationInProgress, "A domain operation is already in progress")
			return
		}
		s.domainMutationActive = true
		s.domainMutationMu.Unlock()
		defer func() {
			s.domainMutationMu.Lock()
			s.domainMutationActive = false
			s.domainMutationMu.Unlock()
		}()
		next.ServeHTTP(w, r)
	})
}
