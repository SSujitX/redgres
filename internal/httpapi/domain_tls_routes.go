package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
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
		if syncErr := s.syncCertbotDNSCredentialsFromAPIToken(); syncErr != nil {
			s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Certbot DNS credentials are not configured")
			return
		}
	}
	client, err := s.cloudflareClientFromConfig(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Cloudflare credentials are not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	ok := false
	finish := s.beginDomainActivity("tls", "issue_tls")
	defer finish(&ok)
	hostnames := []string{dep.DBHostname, dep.rsHostname()}
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.tls.issue", dep.consoleHostname(), "started", requestID(r), requestClientIP(r), map[string]any{"hostname_count": len(hostnames)}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	defer func() {
		if !ok {
			_ = s.audit.Record(sess.Username, "domain.tls.issue", dep.consoleHostname(), "failure", requestID(r), requestClientIP(r), map[string]any{"hostname_count": len(hostnames)})
		}
	}()
	if tlsops.ReadIssueResult(s.cfg.TLSIssueResultFile) == "failed" {
		failure := tlsops.ReadIssueFailure(s.cfg.TLSIssueResultFile)
		if failure.Covers(hostnames) && failure.Code == tlsops.IssueFailureRateLimited && failure.RetryAfter.After(time.Now().UTC()) {
			s.domainActivity.SetFailure("issue_tls", string(failure.Code), failure.RetryAfter.Format(time.RFC3339))
			s.writeError(w, r, http.StatusTooManyRequests, CodeRateLimited, "TLS certificate requests are temporarily limited")
			return
		}
	}
	for _, rec := range dep.Records {
		if err := client.VerifyRecord(ctx, dep.ZoneID, rec.ID); err != nil {
			s.writeCFError(w, r, err)
			return
		}
	}
	if s.cfg.TLSIssueRequestFile != "" {
		queuedAfter := time.Now()
		if err := tlsops.WriteTLSIssueRequest(s.cfg.TLSIssueRequestFile, hostnames); err != nil {
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "TLS issue could not be queued")
			return
		}
		if failure, err := waitTLSIssueResult(ctx, s.cfg.TLSIssueResultFile, hostnames, queuedAfter); err != nil {
			retryAfter := ""
			if !failure.RetryAfter.IsZero() {
				retryAfter = failure.RetryAfter.Format(time.RFC3339)
			}
			s.domainActivity.SetFailure("issue_tls", string(failure.Code), retryAfter)
			dep.TLSStatus = "failed"
			dep.TLSDBStatus = "failed"
			dep.TLSRSStatus = "failed"
			_ = store.Save(ctx, dep)
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "TLS issuance failed")
			return
		}
		statuses := tlsops.ReadIssueEndpointStatuses(s.cfg.TLSIssueResultFile)
		dep.TLSDBStatus = statuses["db"]
		dep.TLSRSStatus = statuses["rs"]
	} else {
		issuer := tlsops.Issuer{Bin: s.cfg.CertbotBin, DNSCredentialsFile: creds}
		if err := issuer.Issue(ctx, hostnames); err != nil {
			s.domainActivity.SetFailure("issue_tls", string(tlsops.IssueFailureDependency), "")
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
	}
	dep.TLSStatus = "issued"
	if dep.TLSDBStatus == "" {
		dep.TLSDBStatus = "certificate_prepared"
	}
	if dep.TLSRSStatus == "" {
		dep.TLSRSStatus = "certificate_prepared"
	}
	if err := store.Save(ctx, dep); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	if err := s.audit.Record(sess.Username, "domain.tls.issue", dep.consoleHostname(), "success", requestID(r), requestClientIP(r), map[string]any{"hostname_count": len(hostnames)}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	ok = true
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"ok":         true,
		"tls":        dep.tlsMap(),
		"request_id": requestID(r),
	})
}

func waitTLSIssueResult(ctx context.Context, path string, hostnames []string, notBefore time.Time) (tlsops.IssueFailure, error) {
	if strings.TrimSpace(path) == "" {
		return tlsops.IssueFailure{Code: tlsops.IssueFailureDependency}, errors.New("tls issue result file is not configured")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if info, err := os.Stat(path); err == nil && !info.ModTime().Before(notBefore) {
			if tlsops.IssueResultCovers(path, hostnames) {
				return tlsops.IssueFailure{}, nil
			}
			if tlsops.ReadIssueResult(path) == "failed" {
				return tlsops.ReadIssueFailure(path), errors.New("tls issue failed")
			}
		}
		select {
		case <-ctx.Done():
			return tlsops.IssueFailure{Code: tlsops.IssueFailureDependency}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitTLSCleanupResult(ctx context.Context, path string, notBefore time.Time) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if info, err := os.Stat(path); err == nil && !info.ModTime().Before(notBefore) {
			raw, readErr := os.ReadFile(path)
			if readErr == nil && string(raw) == "cleaned\n" {
				return nil
			}
			if readErr == nil && string(raw) == "failed\n" {
				return errors.New("tls credential cleanup failed")
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
