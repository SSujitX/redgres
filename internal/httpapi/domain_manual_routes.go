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
	hosts, err := parseDomainHostnames(zone, hostnames["console"], hostnames)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid zone or hostname")
		return
	}
	originIP := strings.TrimSpace(originIPRaw)
	if net.ParseIP(originIP) == nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid origin IP")
		return
	}
	if _, err := (domainStore{s.db}).Get(r.Context()); err == nil {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "A domain is already configured; disconnect first")
		return
	}
	ok := false
	finish := s.beginDomainActivity("manual_apply", "write_instructions", "save_config")
	defer finish(&ok)
	greySpecs, err := cloudflare.GreyCloudRecords(originIP)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid origin IP")
		return
	}
	instructions := manualDNSInstructions(hosts, greySpecs)
	dep := deployment{
		ZoneName:             zone,
		Hostname:             hosts.Console,
		ConsoleHostname:      hosts.Console,
		DBHostname:           hosts.DB,
		RSHostname:           hosts.RS,
		PgAdminHostname:      hosts.PgAdmin,
		RedisInsightHostname: hosts.RedisInsight,
		OriginIP:             originIP,
		TLSStatus:            "not_issued",
		DNSProvider:          "manual",
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	s.domainActivity.Advance("save_config")
	if err := (domainStore{s.db}).Save(ctx, dep); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.activateToolPublicURLs(hosts)
	s.refreshConsoleOrigin(r.Context())
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.manual.apply", hosts.Console, "success", requestID(r), requestClientIP(r), map[string]any{"instruction_count": len(instructions)}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	ok = true
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
	ok := false
	finish := s.beginDomainActivity("confirm_access", "confirm_access")
	defer finish(&ok)
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
	ok = true
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
	for _, host := range []string{dep.DBHostname, dep.rsHostname()} {
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
	hosts := domainHostnames{
		Console:      dep.consoleHostname(),
		DB:           dep.DBHostname,
		RS:           dep.rsHostname(),
		PgAdmin:      dep.PgAdminHostname,
		RedisInsight: dep.RedisInsightHostname,
	}
	return manualDNSInstructions(hosts, greySpecs), nil
}

func manualDNSInstructions(hosts domainHostnames, grey []struct {
	Type    string
	Content string
}) []string {
	var out []string
	for _, host := range []string{hosts.Console, hosts.PgAdmin, hosts.RedisInsight} {
		if host == "" {
			continue
		}
		out = append(out, fmt.Sprintf("Create a proxied CNAME for %s pointing to your cloudflared tunnel hostname.", host))
	}
	for _, host := range []string{hosts.DB, hosts.RS} {
		if host == "" {
			continue
		}
		for _, spec := range grey {
			out = append(out, fmt.Sprintf("Create a DNS-only %s record for %s with content %s.", spec.Type, host, spec.Content))
		}
	}
	tunnelHosts := hosts.Console
	if hosts.PgAdmin != "" {
		tunnelHosts += ", " + hosts.PgAdmin
	}
	if hosts.RedisInsight != "" {
		tunnelHosts += ", " + hosts.RedisInsight
	}
	out = append(out, fmt.Sprintf("Configure Cloudflare Access on %s (deny by default).", tunnelHosts))
	return out
}
