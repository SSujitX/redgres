package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SSujitX/redgres/internal/cloudflare"
	"github.com/SSujitX/redgres/internal/securefile"
)

var errNoDeployment = errors.New("no domain deployment")

// deployment is the persisted, non-secret record of what the wizard created so
// disconnect can delete exactly those resources.
type deployment struct {
	AccountID   string   `json:"account_id"`
	ZoneID      string   `json:"zone_id"`
	ZoneName    string   `json:"zone_name"`
	TunnelID    string   `json:"tunnel_id"`
	AccessAppID string   `json:"access_app_id"`
	Records     []record `json:"records"`
	Hostname    string   `json:"hostname"`
}

type record struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
			s.writeJSON(w, r, http.StatusOK, map[string]any{"configured": false, "request_id": requestID(r)})
			return
		}
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"configured": true,
		"zone":       dep.ZoneName,
		"hostname":   dep.Hostname,
		"request_id": requestID(r),
	})
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
	if s.cfg.CloudflareTokenFile == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Cloudflare token file is not configured")
		return
	}
	if s.cfg.TunnelTokenFile == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Tunnel token file is not configured")
		return
	}
	var body struct {
		Zone     string `json:"zone"`
		Hostname string `json:"hostname"`
	}
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	zone := strings.ToLower(strings.TrimSpace(body.Zone))
	hostname := strings.ToLower(strings.TrimSpace(body.Hostname))
	if !validHostname(zone) || !validHostname(hostname) {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid zone or hostname")
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
		if dep.AccessAppID != "" {
			_ = client.DeleteAccessApp(ctx, z.AccountID, dep.AccessAppID)
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
	tunnel, err := client.CreateTunnel(ctx, z.AccountID, "redgres-"+hostname)
	if err != nil {
		s.writeCFError(w, r, err)
		return
	}
	dep = deployment{AccountID: z.AccountID, ZoneID: z.ID, ZoneName: z.Name, TunnelID: tunnel.ID, Hostname: hostname}

	if err := client.ConfigureIngress(ctx, z.AccountID, tunnel.ID, hostname, origin); err != nil {
		compensate()
		s.writeCFError(w, r, err)
		return
	}

	rec, err := client.CreateRecord(ctx, z.ID, hostname, tunnel.ID+".cfargotunnel.com", true)
	if err != nil {
		compensate()
		s.writeCFError(w, r, err)
		return
	}
	dep.Records = append(dep.Records, record{ID: rec.ID, Name: rec.Name})

	app, err := client.CreateAccessApp(ctx, z.AccountID, hostname)
	if err != nil {
		compensate()
		s.writeCFError(w, r, err)
		return
	}
	dep.AccessAppID = app.ID

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
	if err := client.VerifyAccessApp(ctx, z.AccountID, app.ID); err != nil {
		compensate()
		s.writeCFError(w, r, err)
		return
	}

	// Persist the one-time tunnel connector credential server-side; it is never
	// returned to the browser. cloudflared loads it later from this file.
	if err := writeTokenFile(s.cfg.TunnelTokenFile, tunnel.Token); err != nil {
		compensate()
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "Tunnel token could not be stored")
		return
	}
	if err := (domainStore{s.db}).Save(ctx, dep); err != nil {
		_ = os.Remove(s.cfg.TunnelTokenFile)
		compensate()
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}

	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.apply", hostname, "success", requestID(r), requestClientIP(r), map[string]any{"zone": zone, "hostname_count": 1}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"zone":                 z.Name,
		"hostname":             hostname,
		"tunnel_id":            tunnel.ID,
		"bootstrap_still_open": true,
		"access":               "deny_by_default",
		"request_id":           requestID(r),
	})
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
	token, err := readTokenFile(s.cfg.CloudflareTokenFile)
	if err != nil || token == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Cloudflare token is not configured")
		return
	}
	client := s.cloudflareClient(token)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Delete exactly the persisted resources, in reverse creation order.
	_ = client.DeleteAccessApp(ctx, dep.AccountID, dep.AccessAppID)
	for _, rec := range dep.Records {
		_ = client.DeleteRecord(ctx, dep.ZoneID, rec.ID)
	}
	_ = client.DeleteTunnel(ctx, dep.AccountID, dep.TunnelID)

	if err := store.Clear(ctx); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	if s.cfg.TunnelTokenFile != "" {
		_ = os.Remove(s.cfg.TunnelTokenFile)
	}
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.disconnect", dep.Hostname, "success", requestID(r), requestClientIP(r), map[string]any{"zone": dep.ZoneName}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{"ok": true, "request_id": requestID(r)})
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
	var apiErr *cloudflare.APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
		s.writeError(w, r, http.StatusForbidden, CodeForbidden, "Cloudflare token is unauthorized")
		return
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
	if len(s) > 253 {
		return false
	}
	if strings.ContainsAny(s, " /:@?%#\\") {
		return false
	}
	return strings.Contains(s, ".")
}
