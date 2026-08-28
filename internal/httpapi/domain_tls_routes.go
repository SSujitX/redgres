package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/SSujitX/redgres/internal/tlsops"
)

func (s *Server) handleDomainTLSIssue(w http.ResponseWriter, r *http.Request) {
	store := domainStore{s.db}
	dep, err := store.Get(r.Context())
	if err != nil {
		if errors.Is(err, errNoDeployment) {
			s.writeError(w, r, http.StatusNotFound, CodeNotFound, "No domain configured")
			return
		}
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	if dep.DNSProvider == "manual" {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "TLS issue is not available for manual DNS mode")
		return
	}
	if dep.DBHostname == "" || dep.rsHostname() == "" {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "DB and RS hostnames are required")
		return
	}
	creds := s.cfg.CertbotDNSCredentialsFile
	if creds == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Certbot DNS credentials file is not configured")
		return
	}
	if _, err := os.Stat(creds); err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Certbot DNS credentials are not configured")
		return
	}
	client, err := s.cloudflareClientFromConfig(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Cloudflare credentials are not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	for _, rec := range dep.Records {
		if err := client.VerifyRecord(ctx, dep.ZoneID, rec.ID); err != nil {
			s.writeCFError(w, r, err)
			return
		}
	}
	issuer := tlsops.Issuer{Bin: s.cfg.CertbotBin, DNSCredentialsFile: creds}
	hostnames := []string{dep.DBHostname, dep.rsHostname()}
	if err := issuer.Issue(ctx, hostnames); err != nil {
		dep.TLSStatus = "failed"
		dep.TLSDBStatus = "failed"
		dep.TLSRSStatus = "failed"
		_ = store.Save(ctx, dep)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "TLS issuance failed")
		return
	}
	if err := tlsops.VerifyCertFiles(hostnames, time.Now().UTC()); err != nil {
		dep.TLSStatus = "failed"
		dep.TLSDBStatus = "failed"
		dep.TLSRSStatus = "failed"
		_ = store.Save(ctx, dep)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "TLS verification failed")
		return
	}
	dep.TLSStatus = "issued"
	dep.TLSDBStatus = "issued"
	dep.TLSRSStatus = "issued"
	if err := store.Save(ctx, dep); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.tls.issue", dep.consoleHostname(), "success", requestID(r), requestClientIP(r), map[string]any{"hostname_count": len(hostnames)}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"ok":         true,
		"tls":        dep.tlsMap(),
		"request_id": requestID(r),
	})
}
