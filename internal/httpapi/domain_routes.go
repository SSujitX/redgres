package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/SSujitX/redgres/internal/bootstrap"
	"github.com/SSujitX/redgres/internal/cloudflare"
	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/securefile"
	"github.com/SSujitX/redgres/internal/tlsops"
)

var errNoDeployment = errors.New("no domain deployment")

type accessAppBinding struct {
	Hostname string `json:"hostname,omitempty"`
	AppID    string `json:"app_id"`
	PolicyID string `json:"policy_id,omitempty"`
}

// deployment is the persisted, non-secret record of what the wizard created so
// disconnect can delete exactly those resources.
type deployment struct {
	AccountID             string             `json:"account_id"`
	ZoneID                string             `json:"zone_id"`
	ZoneName              string             `json:"zone_name"`
	TunnelID              string             `json:"tunnel_id"`
	AccessAppID           string             `json:"access_app_id"`
	AccessPolicyID        string             `json:"access_policy_id,omitempty"`
	AccessApps            []accessAppBinding `json:"access_apps,omitempty"`
	Records               []record           `json:"records"`
	Hostname              string             `json:"hostname"` // legacy alias for console
	ConsoleHostname       string             `json:"console_hostname,omitempty"`
	DBHostname            string             `json:"db_hostname,omitempty"`
	RSHostname            string             `json:"rs_hostname,omitempty"`
	RedisHostname         string             `json:"redis_hostname,omitempty"` // legacy raw Redis endpoint
	PgAdminHostname       string             `json:"pgadmin_hostname,omitempty"`
	RedisInsightHostname  string             `json:"redis_insight_hostname,omitempty"`
	OriginIP              string             `json:"origin_ip,omitempty"`
	TLSStatus             string             `json:"tls_status,omitempty"`
	TLSDBStatus           string             `json:"tls_db_status,omitempty"`
	TLSRSStatus           string             `json:"tls_rs_status,omitempty"`
	TLSRedisStatus        string             `json:"tls_redis_status,omitempty"`
	DNSProvider           string             `json:"dns_provider,omitempty"`
	AccessManualConfirmed bool               `json:"access_manual_confirmed,omitempty"`
}

func (d deployment) hasAccessAllowPolicy() bool {
	if d.AccessPolicyID != "" {
		return true
	}
	for _, app := range d.AccessApps {
		if app.PolicyID != "" {
			return true
		}
	}
	return false
}

func (d deployment) accessAllow() bool {
	if d.AccessManualConfirmed {
		return true
	}
	if d.AccessPolicyID != "" {
		return true
	}
	for _, app := range d.AccessApps {
		if app.PolicyID != "" {
			return true
		}
	}
	return false
}

type record struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Proxied bool   `json:"proxied,omitempty"`
}

func (d deployment) consoleHostname() string {
	if d.ConsoleHostname != "" {
		return d.ConsoleHostname
	}
	return d.Hostname
}

type domainStore struct{ db *sql.DB }

func (s domainStore) Get(ctx context.Context) (deployment, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM domain_deployment WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return deployment{}, errNoDeployment
	}
	if err != nil {
		return deployment{}, err
	}
	var d deployment
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return deployment{}, err
	}
	d.normalizeLegacyHostnames()
	return d, nil
}

func (s domainStore) Save(ctx context.Context, d deployment) error {
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO domain_deployment (id, payload, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at`,
		string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s domainStore) Clear(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM domain_deployment WHERE id = 1`)
	return err
}

func (s *Server) handleDomainGet(w http.ResponseWriter, r *http.Request) {
	dep, err := (domainStore{s.db}).Get(r.Context())
	if err != nil {
		if errors.Is(err, errNoDeployment) {
			s.writeJSON(w, r, http.StatusOK, map[string]any{
				"configured":           false,
				"bootstrap_still_open": s.bootstrapOpen(),
				"request_id":           requestID(r),
			})
			return
		}
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	access := "deny_by_default"
	if dep.accessAllow() {
		access = "allow"
	}
	resp := map[string]any{
		"configured":           true,
		"zone":                 dep.ZoneName,
		"hostname":             dep.consoleHostname(),
		"hostnames":            dep.hostnamesMap(),
		"origin_ip":            dep.OriginIP,
		"access":               access,
		"tls":                  s.domainTLSMap(dep),
		"credential":           s.domainCredentialKind(),
		"dns_provider":         dep.DNSProvider,
		"bootstrap_still_open": s.bootstrapOpen(),
		"request_id":           requestID(r),
	}
	if dep.TunnelID != "" {
		resp["tunnel_id"] = dep.TunnelID
	}
	if dep.DNSProvider == "manual" {
		if instructions, err := manualInstructionsForDep(dep); err == nil {
			resp["instructions"] = instructions
		}
	}
	s.writeJSON(w, r, http.StatusOK, resp)
}

func (s *Server) bootstrapOpen() bool {
	return s.bootstrapCloser != nil && s.bootstrapCloser.Open()
}

func (s *Server) handleDomainTokenSet(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CloudflareTokenFile == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Cloudflare token file is not configured")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" || len(token) > 512 || strings.ContainsAny(token, " \t\r\n") {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid token")
		return
	}
	if err := writeTokenFile(s.cfg.CloudflareTokenFile, token); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "Cloudflare token could not be stored")
		return
	}
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.token.set", "", "success", requestID(r), requestClientIP(r), map[string]any{"configured": true}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{"ok": true, "request_id": requestID(r)})
}

func (s *Server) handleDomainApply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Zone        string            `json:"zone"`
		Hostname    string            `json:"hostname"`
		OriginIP    string            `json:"origin_ip"`
		Hostnames   map[string]string `json:"hostnames"`
		DNSProvider string            `json:"dns_provider"`
	}
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	if strings.TrimSpace(body.DNSProvider) == "manual" {
		s.handleDomainManualApplyBody(w, r, body.Zone, body.OriginIP, body.Hostnames)
		return
	}
	if s.cfg.CloudflareTokenFile == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Cloudflare token file is not configured")
		return
	}
	if s.cfg.TunnelTokenFile == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Tunnel token file is not configured")
		return
	}
	zone := strings.ToLower(strings.TrimSpace(body.Zone))
	hosts, err := parseDomainHostnames(zone, body.Hostname, body.Hostnames)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid zone or hostname")
		return
	}
	originIP := strings.TrimSpace(body.OriginIP)
	if net.ParseIP(originIP) == nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid origin IP")
		return
	}
	if _, err := (domainStore{s.db}).Get(r.Context()); err == nil {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "A domain is already configured; disconnect first")
		return
	}

	token, err := readTokenFile(s.cfg.CloudflareTokenFile)
	if err != nil || token == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Cloudflare token is not configured")
		return
	}
	client := s.cloudflareClient(token)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	z, err := client.DiscoverZone(ctx, zone)
	if err != nil {
		s.writeCFError(w, r, err)
		return
	}

	// Delete whatever was created (reverse creation order, same as disconnect)
	// on any failure after the first resource is created.
	var dep deployment
	compensate := func() {
		deletedApps := map[string]struct{}{}
		for _, binding := range dep.AccessApps {
			if binding.AppID == "" {
				continue
			}
			if _, ok := deletedApps[binding.AppID]; ok {
				continue
			}
			_ = client.DeleteAccessApp(ctx, z.AccountID, binding.AppID)
			deletedApps[binding.AppID] = struct{}{}
		}
		if dep.AccessAppID != "" {
			if _, ok := deletedApps[dep.AccessAppID]; !ok {
				_ = client.DeleteAccessApp(ctx, z.AccountID, dep.AccessAppID)
			}
		}
		for _, rec := range dep.Records {
			_ = client.DeleteRecord(ctx, z.ID, rec.ID)
		}
		if dep.TunnelID != "" {
			_ = client.DeleteTunnel(ctx, z.AccountID, dep.TunnelID)
		}
	}

	origin, err := cloudflare.OriginHTTPService(s.cfg.Address)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Internal server error")
		return
	}
	tunnel, err := client.CreateTunnel(ctx, z.AccountID, "redgres-"+hosts.Console)
	if err != nil {
		s.writeCFError(w, r, err)
		return
	}
	dep = deployment{
		AccountID:            z.AccountID,
		ZoneID:               z.ID,
		ZoneName:             z.Name,
		TunnelID:             tunnel.ID,
		Hostname:             hosts.Console,
		ConsoleHostname:      hosts.Console,
		DBHostname:           hosts.DB,
		RSHostname:           hosts.RS,
		PgAdminHostname:      hosts.PgAdmin,
		RedisInsightHostname: hosts.RedisInsight,
		OriginIP:             originIP,
		TLSStatus:            "not_issued",
		DNSProvider:          "cloudflare",
	}

	ingressRoutes := []cloudflare.IngressRoute{{Hostname: hosts.Console, Service: origin}}
	if hosts.PgAdmin != "" {
		ingressRoutes = append(ingressRoutes, cloudflare.IngressRoute{Hostname: hosts.PgAdmin, Service: defaultPgAdminTunnelOrigin})
	}
	if hosts.RedisInsight != "" {
		ingressRoutes = append(ingressRoutes, cloudflare.IngressRoute{Hostname: hosts.RedisInsight, Service: defaultRedisInsightTunnelOrigin})
	}
	if err := client.ConfigureIngressRoutes(ctx, z.AccountID, tunnel.ID, ingressRoutes); err != nil {
		compensate()
		s.writeCFError(w, r, err)
		return
	}

	rec, err := client.CreateRecord(ctx, z.ID, hosts.Console, tunnel.ID+".cfargotunnel.com", true)
	if err != nil {
		compensate()
		s.writeCFError(w, r, err)
		return
	}
	dep.Records = append(dep.Records, record{ID: rec.ID, Name: rec.Name, Type: "CNAME", Proxied: true})

	greySpecs, err := cloudflare.GreyCloudRecords(originIP)
	if err != nil {
		compensate()
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid origin IP")
		return
	}
	for _, host := range []string{hosts.DB, hosts.RS} {
		for _, spec := range greySpecs {
			grec, err := client.CreateDNSRecord(ctx, z.ID, host, spec.Type, spec.Content, false)
			if err != nil {
				compensate()
				s.writeCFError(w, r, err)
				return
			}
			dep.Records = append(dep.Records, record{ID: grec.ID, Name: grec.Name, Type: spec.Type, Proxied: false})
		}
	}

	for _, tunnelHost := range []string{hosts.PgAdmin, hosts.RedisInsight} {
		if tunnelHost == "" {
			continue
		}
		trec, err := client.CreateRecord(ctx, z.ID, tunnelHost, tunnel.ID+".cfargotunnel.com", true)
		if err != nil {
			compensate()
			s.writeCFError(w, r, err)
			return
		}
		dep.Records = append(dep.Records, record{ID: trec.ID, Name: trec.Name, Type: "CNAME", Proxied: true})
	}

	for _, tunnelHost := range dep.tunnelHostnames() {
		app, err := client.CreateAccessApp(ctx, z.AccountID, tunnelHost)
		if err != nil {
			compensate()
			s.writeCFError(w, r, err)
			return
		}
		dep.AccessApps = append(dep.AccessApps, accessAppBinding{Hostname: tunnelHost, AppID: app.ID})
		if dep.AccessAppID == "" {
			dep.AccessAppID = app.ID
		}
	}

	app := cloudflare.AccessApp{ID: dep.AccessAppID}
	if dep.AccessAppID == "" {
		compensate()
		s.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Internal server error")
		return
	}

	// Verify the resources exist in Cloudflare (API reflection only until
	// cloudflared + an Access policy are wired later).
	if err := client.VerifyTunnel(ctx, z.AccountID, tunnel.ID); err != nil {
		compensate()
		s.writeCFError(w, r, err)
		return
	}
	if err := client.VerifyRecord(ctx, z.ID, rec.ID); err != nil {
		compensate()
		s.writeCFError(w, r, err)
		return
	}
	for _, persisted := range dep.Records[1:] {
		if err := client.VerifyRecord(ctx, z.ID, persisted.ID); err != nil {
			compensate()
			s.writeCFError(w, r, err)
			return
		}
	}
	if err := client.VerifyAccessApp(ctx, z.AccountID, app.ID); err != nil {
		compensate()
		s.writeCFError(w, r, err)
		return
	}
	for _, binding := range dep.AccessApps[1:] {
		if err := client.VerifyAccessApp(ctx, z.AccountID, binding.AppID); err != nil {
			compensate()
			s.writeCFError(w, r, err)
			return
		}
	}

	// Persist the one-time tunnel connector credential server-side; it is never
	// returned to the browser. cloudflared loads it later from this file.
	if err := writeTokenFile(s.cfg.TunnelTokenFile, tunnel.Token); err != nil {
		compensate()
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "Tunnel token could not be stored")
		return
	}
	if s.cfg.CertbotDNSCredentialsFile != "" {
		if err := s.syncCertbotDNSCredentialsFromAPIToken(); err != nil {
			_ = os.Remove(s.cfg.TunnelTokenFile)
			compensate()
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "Certbot DNS credentials could not be stored")
			return
		}
	}
	if s.cfg.TLSIssueResultFile != "" {
		_ = os.Remove(s.cfg.TLSIssueResultFile)
	}
	if s.cfg.TLSIssueRequestFile != "" {
		if err := tlsops.WriteTLSIssueRequest(s.cfg.TLSIssueRequestFile, []string{hosts.DB, hosts.RS}); err != nil {
			_ = os.Remove(s.cfg.TunnelTokenFile)
			compensate()
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "TLS issue could not be queued")
			return
		}
	}
	if err := (domainStore{s.db}).Save(ctx, dep); err != nil {
		_ = os.Remove(s.cfg.TunnelTokenFile)
		if s.cfg.TLSIssueRequestFile != "" {
			_ = os.Remove(s.cfg.TLSIssueRequestFile)
		}
		compensate()
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.activateToolPublicURLs(hosts)
	s.refreshConsoleOrigin(r.Context())

	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.apply", hosts.Console, "success", requestID(r), requestClientIP(r), map[string]any{"zone": zone, "hostname_count": len(dep.hostnamesMap())}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.log.Info("domain.apply",
		slog.String("request_id", requestID(r)),
		slog.String("zone", zone),
		slog.String("hostname", hosts.Console),
		slog.Int("hostname_count", len(dep.hostnamesMap())),
		slog.String("access", "deny_by_default"),
		slog.Bool("bootstrap_still_open", s.bootstrapOpen()),
	)
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"zone":                 z.Name,
		"hostname":             hosts.Console,
		"hostnames":            dep.hostnamesMap(),
		"origin_ip":            originIP,
		"tunnel_id":            tunnel.ID,
		"bootstrap_still_open": s.bootstrapOpen(),
		"access":               "deny_by_default",
		"tls":                  dep.tlsMap(),
		"request_id":           requestID(r),
	})
}

func (s *Server) handleDomainAccessPolicy(w http.ResponseWriter, r *http.Request) {
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
		s.writeError(w, r, http.StatusConflict, CodeConflict, "Manual DNS mode uses POST /api/v1/domain/manual/confirm-access")
		return
	}
	if dep.AccessManualConfirmed {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "Manual Access is already confirmed")
		return
	}
	if dep.hasAccessAllowPolicy() {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "An Access allow policy is already configured")
		return
	}
	var body struct {
		Emails []string `json:"emails"`
	}
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	emails, err := normalizeAccessEmails(body.Emails)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, err.Error())
		return
	}
	token, err := s.resolveCloudflareBearer(r.Context())
	if err != nil || token == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Cloudflare credentials are not configured")
		return
	}
	client := s.cloudflareClient(token)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	policy, err := client.CreateAccessPolicy(ctx, dep.AccountID, dep.AccessAppID, emails)
	if err != nil {
		s.writeCFError(w, r, err)
		return
	}
	if err := client.VerifyAccessPolicy(ctx, dep.AccountID, dep.AccessAppID, policy.ID); err != nil {
		_ = client.DeleteAccessPolicy(ctx, dep.AccountID, dep.AccessAppID, policy.ID)
		s.writeCFError(w, r, err)
		return
	}
	dep.AccessPolicyID = policy.ID
	for i := range dep.AccessApps {
		if dep.AccessApps[i].AppID == dep.AccessAppID {
			dep.AccessApps[i].PolicyID = policy.ID
			continue
		}
		if dep.AccessApps[i].PolicyID != "" {
			continue
		}
		extra, err := client.CreateAccessPolicy(ctx, dep.AccountID, dep.AccessApps[i].AppID, emails)
		if err != nil {
			_ = client.DeleteAccessPolicy(ctx, dep.AccountID, dep.AccessAppID, policy.ID)
			s.writeCFError(w, r, err)
			return
		}
		if err := client.VerifyAccessPolicy(ctx, dep.AccountID, dep.AccessApps[i].AppID, extra.ID); err != nil {
			_ = client.DeleteAccessPolicy(ctx, dep.AccountID, dep.AccessApps[i].AppID, extra.ID)
			_ = client.DeleteAccessPolicy(ctx, dep.AccountID, dep.AccessAppID, policy.ID)
			s.writeCFError(w, r, err)
			return
		}
		dep.AccessApps[i].PolicyID = extra.ID
	}
	if err := store.Save(ctx, dep); err != nil {
		_ = client.DeleteAccessPolicy(ctx, dep.AccountID, dep.AccessAppID, policy.ID)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.access.allow", dep.consoleHostname(), "success", requestID(r), requestClientIP(r), map[string]any{"email_count": len(emails)}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.log.Info("domain.access.allow",
		slog.String("request_id", requestID(r)),
		slog.String("hostname", dep.consoleHostname()),
		slog.Int("email_count", len(emails)),
	)
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"ok":                   true,
		"access":               "allow",
		"bootstrap_still_open": s.bootstrapOpen(),
		"request_id":           requestID(r),
	})
}

func (s *Server) handleDomainConfirmReachable(w http.ResponseWriter, r *http.Request) {
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
	if !dep.accessAllow() {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "Configure Access before closing bootstrap")
		return
	}
	willClose := s.bootstrapCloser != nil && s.bootstrapCloser.Open()
	ufwCmd := strings.TrimSpace(s.cfg.BootstrapUFWRemoveCmd)
	envFile := s.cfg.EnvFile
	if hostname := dep.consoleHostname(); hostname != "" {
		origin := "https://" + hostname
		rewritten, err := config.PersistBootstrapClosed(envFile, origin)
		if err != nil {
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "Bootstrap close could not persist")
			return
		}
		if s.cfg.Production() && !rewritten {
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "Bootstrap close could not persist")
			return
		}
		if rewritten {
			s.cfg.BaseURL = origin
			s.cfg.CookieSecure = true
			s.cfg.BootstrapAddress = ""
		}
		s.refreshConsoleOrigin(r.Context())
	}
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.confirm_reachable", dep.consoleHostname(), "success", requestID(r), requestClientIP(r), map[string]any{"bootstrap_closed": willClose}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	_ = bootstrap.WriteClosedState(s.cfg.SQLitePath)
	ufwRemoved := false
	if ufwCmd != "" {
		ufwCtx, ufwCancel := context.WithTimeout(r.Context(), 5*time.Second)
		if err := exec.CommandContext(ufwCtx, ufwCmd).Run(); err == nil {
			ufwRemoved = true
		}
		ufwCancel()
	}
	// Schedule graceful shutdown after persist so a persist failure does not
	// close :8989. Shutdown waits for this handler to finish writing the body
	// (force Close remains the hard-cap timer path).
	if willClose {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = s.bootstrapCloser.Shutdown(ctx)
		}()
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"ok":                      true,
		"bootstrap_still_open":    !willClose && s.bootstrapOpen(),
		"bootstrap_closed":        willClose,
		"bootstrap_ufw_removed":   ufwRemoved,
		"bootstrap_ufw_attempted": ufwCmd != "",
		"request_id":              requestID(r),
	})
}

func normalizeAccessEmails(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, errors.New("At least one email is required")
	}
	if len(raw) > 8 {
		return nil, errors.New("At most 8 emails are allowed")
	}
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		email := strings.TrimSpace(strings.ToLower(item))
		if email == "" || len(email) > 254 || strings.ContainsAny(email, " \t\r\n") {
			return nil, errors.New("Invalid email")
		}
		at := strings.IndexByte(email, '@')
		if at < 1 || at == len(email)-1 || strings.Count(email, "@") != 1 {
			return nil, errors.New("Invalid email")
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		out = append(out, email)
	}
	if len(out) == 0 {
		return nil, errors.New("At least one email is required")
	}
	return out, nil
}

func (s *Server) handleDomainDisconnect(w http.ResponseWriter, r *http.Request) {
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
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if dep.DNSProvider != "manual" {
		token, err := s.resolveCloudflareBearer(ctx)
		if err != nil || token == "" {
			s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Cloudflare credentials are not configured")
			return
		}
		client := s.cloudflareClient(token)
		deletedApps := map[string]struct{}{}
		for _, binding := range dep.accessAppsForDisconnect() {
			if binding.PolicyID != "" {
				_ = client.DeleteAccessPolicy(ctx, dep.AccountID, binding.AppID, binding.PolicyID)
			}
			if binding.AppID == "" {
				continue
			}
			if _, ok := deletedApps[binding.AppID]; ok {
				continue
			}
			_ = client.DeleteAccessApp(ctx, dep.AccountID, binding.AppID)
			deletedApps[binding.AppID] = struct{}{}
		}
		for _, rec := range dep.Records {
			_ = client.DeleteRecord(ctx, dep.ZoneID, rec.ID)
		}
		if dep.TunnelID != "" {
			_ = client.DeleteTunnel(ctx, dep.AccountID, dep.TunnelID)
		}
	}

	s.revokeOAuthIfPresent(ctx)
	if err := store.Clear(ctx); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	if s.cfg.TunnelTokenFile != "" {
		_ = os.Remove(s.cfg.TunnelTokenFile)
	}
	s.removeCloudflareCredentialFiles()
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.disconnect", dep.consoleHostname(), "success", requestID(r), requestClientIP(r), map[string]any{"zone": dep.ZoneName}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{"ok": true, "request_id": requestID(r)})
}

func (s *Server) originAllowed(r *http.Request) bool {
	bases := []string{s.cfg.BaseURL}
	if extra := s.configuredConsoleOrigin(r.Context()); extra != "" {
		bases = append(bases, extra)
	}
	return auth.SameOriginAny(r.Header.Get("Origin"), r.Header.Get("Referer"), bases...)
}

func (s *Server) configuredConsoleOrigin(ctx context.Context) string {
	if s.db == nil {
		return ""
	}
	dep, err := (domainStore{s.db}).Get(ctx)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(dep.consoleHostname()))
	if host == "" || !validHostname(host) {
		return ""
	}
	return "https://" + host
}

func (s *Server) cloudflareClient(token string) cloudflare.Client {
	if s.cloudflare != nil {
		return s.cloudflare
	}
	return &cloudflare.HTTPClient{Token: token}
}

func (s *Server) writeCFError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, cloudflare.ErrNotFound) {
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Cloudflare resource not found")
		return
	}
	if errors.Is(err, cloudflare.ErrTunnelNameInUse) {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "A Cloudflare tunnel with this hostname already exists")
		return
	}
	var apiErr *cloudflare.APIError
	if errors.As(err, &apiErr) {
		s.log.Warn("domain.cloudflare",
			slog.String("request_id", requestID(r)),
			slog.Int("status", apiErr.StatusCode),
			slog.Int("code", apiErr.Code),
		)
		if apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden {
			s.writeError(w, r, http.StatusForbidden, CodeForbidden, "Cloudflare token is unauthorized")
			return
		}
	}
	s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "Cloudflare API error")
}

func writeTokenFile(path, token string) error {
	f, err := securefile.OpenRegular(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Write([]byte(token)); err != nil {
		return err
	}
	return f.Sync()
}

func readTokenFile(path string) (string, error) {
	raw, err := securefile.ReadRegular(path, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func validHostname(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) == 0 || len(s) > 253 || !strings.Contains(s, ".") {
		return false
	}
	if strings.ContainsAny(s, " /:@?%#\\*") {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func (s *Server) domainTLSMap(dep deployment) map[string]string {
	tls := dep.tlsMap()
	if tlsops.IssueResultCovers(s.cfg.TLSIssueResultFile, []string{dep.DBHostname, dep.rsHostname()}) {
		tls["db"] = "issued"
		tls["rs"] = "issued"
	}
	return tls
}

func (s *Server) syncCertbotDNSCredentialsFromAPIToken() error {
	if s.cfg.CertbotDNSCredentialsFile == "" {
		return errors.New("certbot dns credentials file is not configured")
	}
	token, err := readTokenFile(s.cfg.CloudflareTokenFile)
	if err != nil || token == "" {
		return errors.New("cloudflare token is not configured")
	}
	return tlsops.WriteCloudflareDNSCredentials(s.cfg.CertbotDNSCredentialsFile, token)
}

func (s *Server) domainCredentialKind() string {
	if s.cfg.CloudflareOAuthTokenFile != "" {
		if _, err := os.Stat(s.cfg.CloudflareOAuthTokenFile); err == nil {
			return "oauth"
		}
	}
	if s.cfg.CloudflareTokenFile != "" {
		if tok, err := readTokenFile(s.cfg.CloudflareTokenFile); err == nil && tok != "" {
			return "api_token"
		}
	}
	return "none"
}
