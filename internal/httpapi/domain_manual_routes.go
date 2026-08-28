package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/SSujitX/redgres/internal/cloudflare"
)

func (s *Server) handleDomainManualApplyBody(w http.ResponseWriter, r *http.Request, zoneRaw, originIPRaw string, hostnames map[string]string) {
	zone := strings.ToLower(strings.TrimSpace(zoneRaw))
	console := strings.ToLower(strings.TrimSpace(hostnames["console"]))
	dbHost := strings.ToLower(strings.TrimSpace(hostnames["db"]))
	redisHost := strings.ToLower(strings.TrimSpace(hostnames["redis"]))
	if console == "" && zone != "" {
		console = "console." + zone
	}
	if dbHost == "" && zone != "" {
		dbHost = "db." + zone
	}
	if redisHost == "" && zone != "" {
		redisHost = "redis." + zone
	}
	originIP := strings.TrimSpace(originIPRaw)
	if !validHostname(zone) || !validHostname(console) || !validHostname(dbHost) || !validHostname(redisHost) {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid zone or hostname")
		return
	}
	if net.ParseIP(originIP) == nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid origin IP")
		return
	}
	if _, err := (domainStore{s.db}).Get(r.Context()); err == nil {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "A domain is already configured; disconnect first")
		return
	}
	greySpecs, err := cloudflare.GreyCloudRecords(originIP)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid origin IP")
		return
	}
	instructions := manualDNSInstructions(console, dbHost, redisHost, greySpecs)
	dep := deployment{
		ZoneName:        zone,
		Hostname:        console,
		ConsoleHostname: console,
		DBHostname:      dbHost,
		RedisHostname:   redisHost,
		OriginIP:        originIP,
		TLSStatus:       "not_issued",
		DNSProvider:     "manual",
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := (domainStore{s.db}).Save(ctx, dep); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.manual.apply", console, "success", requestID(r), requestClientIP(r), map[string]any{"instruction_count": len(instructions)}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"zone":                 zone,
		"hostnames":            dep.hostnamesMap(),
		"origin_ip":            originIP,
		"instructions":         instructions,
		"dns_provider":         "manual",
		"bootstrap_still_open": s.bootstrapOpen(),
		"access":               "deny_by_default",
		"request_id":           requestID(r),
	})
}

func (s *Server) handleDomainManualConfirmAccess(w http.ResponseWriter, r *http.Request) {
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
	if dep.DNSProvider != "manual" {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "Manual Access confirmation is only for manual DNS mode")
		return
	}
	if dep.AccessManualConfirmed {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "Manual Access is already confirmed")
		return
	}
	dep.AccessManualConfirmed = true
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := store.Save(ctx, dep); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.manual.access", dep.consoleHostname(), "success", requestID(r), requestClientIP(r), map[string]any{"confirmed": true}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"ok":                   true,
		"access":               "allow",
		"bootstrap_still_open": s.bootstrapOpen(),
		"request_id":           requestID(r),
	})
}

func (s *Server) handleDomainManualVerify(w http.ResponseWriter, r *http.Request) {
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
	if dep.DNSProvider != "manual" {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "Manual verify is only for manual DNS mode")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	results := map[string]string{}
	originIP := net.ParseIP(dep.OriginIP)
	for _, host := range []string{dep.DBHostname, dep.RedisHostname} {
		addrs, err := net.DefaultResolver.LookupHost(ctx, host)
		if err != nil || len(addrs) == 0 {
			results[host] = "missing"
			continue
		}
		found := false
		for _, addr := range addrs {
			if parsed := net.ParseIP(addr); parsed != nil && originIP != nil && parsed.Equal(originIP) {
				found = true
				break
			}
		}
		if found {
			results[host] = "ok"
		} else {
			results[host] = "mismatch"
		}
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"results":    results,
		"request_id": requestID(r),
	})
}

func manualInstructionsForDep(dep deployment) ([]string, error) {
	if dep.DNSProvider != "manual" {
		return nil, nil
	}
	greySpecs, err := cloudflare.GreyCloudRecords(dep.OriginIP)
	if err != nil {
		return nil, err
	}
	return manualDNSInstructions(dep.consoleHostname(), dep.DBHostname, dep.RedisHostname, greySpecs), nil
}

func manualDNSInstructions(console, dbHost, redisHost string, grey []struct {
	Type    string
	Content string
}) []string {
	var out []string
	out = append(out, fmt.Sprintf("Create a proxied CNAME for %s pointing to your cloudflared tunnel hostname.", console))
	for _, host := range []string{dbHost, redisHost} {
		for _, spec := range grey {
			out = append(out, fmt.Sprintf("Create a DNS-only %s record for %s with content %s.", spec.Type, host, spec.Content))
		}
	}
	out = append(out, fmt.Sprintf("Configure Cloudflare Access on %s (deny by default).", console))
	return out
}
