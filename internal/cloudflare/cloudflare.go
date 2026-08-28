// Package cloudflare is a minimal, typed client for the Cloudflare API surface
// the Redgres Domain & Network wizard needs (PRD OPS-009, token-first). It is
// deliberately small: zone discovery, tunnels, DNS records, and Access
// applications. Live requests are not exercised by the test suite (a fake is
// used), so endpoint shapes are documented here and must be re-verified against
// Cloudflare before production sign-off.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// BaseURL is the Cloudflare v4 API root.
const BaseURL = "https://api.cloudflare.com/client/v4"

// ErrNotFound is returned when Cloudflare reports the resource does not exist.
var ErrNotFound = errors.New("cloudflare resource not found")

// ErrTunnelNameInUse is returned when a non-orphan tunnel already uses the name.
var ErrTunnelNameInUse = errors.New("cloudflare tunnel name already in use")

// Zone is the minimal zone identity the wizard needs.
type Zone struct {
	ID        string
	Name      string
	AccountID string
}

// Tunnel is a created Cloudflare tunnel. Token is the one-time connector
// credential Cloudflare returns at creation (not retrievable later).
type Tunnel struct {
	ID    string
	Name  string
	Token string
}

// Record is a created DNS record.
type Record struct {
	ID      string
	Name    string
	Type    string
	Proxied bool
}

// AccessApp is a created Cloudflare Access application.
type AccessApp struct {
	ID     string
	Domain string
}

// AccessPolicy is an allow policy attached to an Access application.
type AccessPolicy struct {
	ID   string
	Name string
}

// Client is the Cloudflare operations the wizard needs. A fake implements this
// for tests; HTTPClient implements it against the live API.
type Client interface {
	DiscoverZone(ctx context.Context, name string) (Zone, error)
	CreateTunnel(ctx context.Context, accountID, name string) (Tunnel, error)
	ConfigureIngress(ctx context.Context, accountID, tunnelID, hostname, originHTTP string) error
	ConfigureIngressRoutes(ctx context.Context, accountID, tunnelID string, routes []IngressRoute) error
	CreateRecord(ctx context.Context, zoneID, name, content string, proxied bool) (Record, error)
	CreateDNSRecord(ctx context.Context, zoneID, name, recordType, content string, proxied bool) (Record, error)
	CreateAccessApp(ctx context.Context, accountID, domain string) (AccessApp, error)
	CreateAccessPolicy(ctx context.Context, accountID, appID string, emails []string) (AccessPolicy, error)
	DeleteTunnel(ctx context.Context, accountID, tunnelID string) error
	DeleteRecord(ctx context.Context, zoneID, recordID string) error
	DeleteAccessApp(ctx context.Context, accountID, appID string) error
	DeleteAccessPolicy(ctx context.Context, accountID, appID, policyID string) error
	VerifyTunnel(ctx context.Context, accountID, tunnelID string) error
	VerifyRecord(ctx context.Context, zoneID, recordID string) error
	VerifyAccessApp(ctx context.Context, accountID, appID string) error
	VerifyAccessPolicy(ctx context.Context, accountID, appID, policyID string) error
}

// APIError is a Cloudflare API error envelope.
type APIError struct {
	StatusCode int
	Code       int
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("cloudflare api %d (%d): %s", e.StatusCode, e.Code, e.Message)
}

// HTTPClient calls the live Cloudflare API with a bearer token.
type HTTPClient struct {
	Token   string
	BaseURL string
	Client  *http.Client
}

func (c *HTTPClient) base() string {
	if c.BaseURL == "" {
		return BaseURL
	}
	return strings.TrimSuffix(c.BaseURL, "/")
}

func (c *HTTPClient) http() *http.Client {
	if c.Client == nil {
		return http.DefaultClient
	}
	return c.Client
}

func (c *HTTPClient) call(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var env struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env); err != nil {
		return err
	}
	if !env.Success {
		code, message := 0, "unknown error"
		if len(env.Errors) > 0 {
			code, message = env.Errors[0].Code, env.Errors[0].Message
		}
		if resp.StatusCode == http.StatusNotFound || code == 1038 || code == 1002 {
			return ErrNotFound
		}
		return &APIError{StatusCode: resp.StatusCode, Code: code, Message: message}
	}
	if out != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return err
		}
	}
	return nil
}

// DiscoverZone resolves a zone by name and returns its ID and account ID.
func (c *HTTPClient) DiscoverZone(ctx context.Context, name string) (Zone, error) {
	var result []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Acct struct {
			ID string `json:"id"`
		} `json:"account"`
	}
	if err := c.call(ctx, http.MethodGet, "/zones?name="+url.QueryEscape(name), nil, &result); err != nil {
		return Zone{}, err
	}
	for _, z := range result {
		if strings.EqualFold(z.Name, name) {
			return Zone{ID: z.ID, Name: z.Name, AccountID: z.Acct.ID}, nil
		}
	}
	return Zone{}, ErrNotFound
}

// CreateTunnel creates a remotely managed tunnel (config_src=cloudflare).
// The one-time connector token is returned in result.token and must be stored
// server-side for cloudflared (never returned to the browser).
// Primary source: https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/get-started/create-remote-tunnel-api/
func (c *HTTPClient) CreateTunnel(ctx context.Context, accountID, name string) (Tunnel, error) {
	t, err := c.createTunnelOnce(ctx, accountID, name)
	if err == nil {
		return t, nil
	}
	if !isTunnelNameConflict(err) {
		return Tunnel{}, err
	}
	if delErr := c.deleteOrphanTunnelByName(ctx, accountID, name); delErr != nil {
		if errors.Is(delErr, ErrTunnelNameInUse) {
			return Tunnel{}, err
		}
		return Tunnel{}, err
	}
	return c.createTunnelOnce(ctx, accountID, name)
}

type tunnelListItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Connections []any  `json:"connections"`
}

func (c *HTTPClient) listTunnelsByName(ctx context.Context, accountID, name string) ([]tunnelListItem, error) {
	var result []tunnelListItem
	path := "/accounts/" + accountID + "/cfd_tunnel?name=" + url.QueryEscape(name)
	if err := c.call(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func tunnelOrphan(item tunnelListItem) bool {
	if item.ID == "" || len(item.Connections) > 0 {
		return false
	}
	return strings.EqualFold(item.Status, "inactive") || item.Status == ""
}

func (c *HTTPClient) deleteOrphanTunnelByName(ctx context.Context, accountID, name string) error {
	tunnels, err := c.listTunnelsByName(ctx, accountID, name)
	if err != nil {
		return err
	}
	var orphans []tunnelListItem
	for _, item := range tunnels {
		if item.Name != name {
			continue
		}
		if tunnelOrphan(item) {
			orphans = append(orphans, item)
			continue
		}
		return ErrTunnelNameInUse
	}
	var lastErr error
	for _, item := range orphans {
		if err := c.DeleteTunnel(ctx, accountID, item.ID); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (c *HTTPClient) createTunnelOnce(ctx context.Context, accountID, name string) (Tunnel, error) {
	var result struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	body := map[string]string{"name": name, "config_src": "cloudflare"}
	if err := c.call(ctx, http.MethodPost, "/accounts/"+accountID+"/cfd_tunnel", body, &result); err != nil {
		return Tunnel{}, err
	}
	if result.ID == "" || result.Token == "" {
		return Tunnel{}, errors.New("cloudflare tunnel create returned empty id or token")
	}
	return Tunnel{ID: result.ID, Name: result.Name, Token: result.Token}, nil
}

func isTunnelNameConflict(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode == http.StatusConflict {
		return true
	}
	return strings.Contains(strings.ToLower(apiErr.Message), "already exists")
}

// IngressRoute is one hostname → loopback HTTP service pair for tunnel ingress.
type IngressRoute struct {
	Hostname string
	Service  string
}

// ConfigureIngress sets remote tunnel ingress for one or more hostnames plus the
// required catch-all http_status:404 rule.
func (c *HTTPClient) ConfigureIngress(ctx context.Context, accountID, tunnelID, hostname, originHTTP string) error {
	return c.ConfigureIngressRoutes(ctx, accountID, tunnelID, []IngressRoute{{Hostname: hostname, Service: originHTTP}})
}

func (c *HTTPClient) ConfigureIngressRoutes(ctx context.Context, accountID, tunnelID string, routes []IngressRoute) error {
	if len(routes) == 0 {
		return errors.New("invalid tunnel ingress hostname or origin")
	}
	ingress := make([]map[string]any, 0, len(routes)+1)
	for _, route := range routes {
		hostname := strings.TrimSpace(strings.ToLower(route.Hostname))
		originHTTP := strings.TrimSpace(route.Service)
		if hostname == "" || !strings.HasPrefix(originHTTP, "http://") {
			return errors.New("invalid tunnel ingress hostname or origin")
		}
		ingress = append(ingress, map[string]any{
			"hostname":      hostname,
			"service":       originHTTP,
			"originRequest": map[string]any{},
		})
	}
	ingress = append(ingress, map[string]any{"service": "http_status:404"})
	body := map[string]any{
		"config": map[string]any{
			"ingress": ingress,
		},
	}
	var result any
	return c.call(ctx, http.MethodPut, "/accounts/"+accountID+"/cfd_tunnel/"+tunnelID+"/configurations", body, &result)
}

// OriginHTTPService builds the local origin URL cloudflared dials on this host.
// Always uses 127.0.0.1 with the port from REDGRES_ADDRESS (loopback UI bind).
func OriginHTTPService(listenAddr string) (string, error) {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil || port == "" {
		return "", errors.New("invalid listen address for tunnel origin")
	}
	return "http://127.0.0.1:" + port, nil
}

// DeleteTunnel deletes a tunnel.
func (c *HTTPClient) DeleteTunnel(ctx context.Context, accountID, tunnelID string) error {
	var result any
	return c.call(ctx, http.MethodDelete, "/accounts/"+accountID+"/cfd_tunnel/"+tunnelID, nil, &result)
}

// VerifyTunnel checks a tunnel still exists.
func (c *HTTPClient) VerifyTunnel(ctx context.Context, accountID, tunnelID string) error {
	var result struct {
		ID string `json:"id"`
	}
	return c.call(ctx, http.MethodGet, "/accounts/"+accountID+"/cfd_tunnel/"+tunnelID, nil, &result)
}

// CreateRecord creates a proxied or grey-cloud CNAME record.
func (c *HTTPClient) CreateRecord(ctx context.Context, zoneID, name, content string, proxied bool) (Record, error) {
	return c.CreateDNSRecord(ctx, zoneID, name, "CNAME", content, proxied)
}

// CreateDNSRecord creates a DNS record of the given type (CNAME, A, or AAAA).
func (c *HTTPClient) CreateDNSRecord(ctx context.Context, zoneID, name, recordType, content string, proxied bool) (Record, error) {
	recordType = strings.ToUpper(strings.TrimSpace(recordType))
	switch recordType {
	case "CNAME", "A", "AAAA":
	default:
		return Record{}, errors.New("unsupported dns record type")
	}
	content = strings.TrimSpace(content)
	if name == "" || content == "" {
		return Record{}, errors.New("invalid dns record name or content")
	}
	var result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	body := map[string]any{
		"type":    recordType,
		"name":    name,
		"content": content,
		"proxied": proxied,
		"ttl":     1,
	}
	if err := c.call(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", body, &result); err != nil {
		return Record{}, err
	}
	return Record{ID: result.ID, Name: result.Name, Type: recordType, Proxied: proxied}, nil
}

// DeleteRecord deletes a DNS record.
func (c *HTTPClient) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	var result any
	return c.call(ctx, http.MethodDelete, "/zones/"+zoneID+"/dns_records/"+recordID, nil, &result)
}

// VerifyRecord checks a DNS record still exists.
func (c *HTTPClient) VerifyRecord(ctx context.Context, zoneID, recordID string) error {
	var result struct {
		ID string `json:"id"`
	}
	return c.call(ctx, http.MethodGet, "/zones/"+zoneID+"/dns_records/"+recordID, nil, &result)
}

// CreateAccessApp creates a self-hosted Access application (deny-by-default
// until an operator adds an identity policy).
func (c *HTTPClient) CreateAccessApp(ctx context.Context, accountID, domain string) (AccessApp, error) {
	var result struct {
		ID     string `json:"id"`
		Domain string `json:"domain"`
	}
	body := map[string]string{"name": domain, "domain": domain, "type": "self_hosted"}
	if err := c.call(ctx, http.MethodPost, "/accounts/"+accountID+"/access/apps", body, &result); err != nil {
		return AccessApp{}, err
	}
	return AccessApp{ID: result.ID, Domain: result.Domain}, nil
}

// CreateAccessPolicy attaches an allow policy for exact emails to an Access app.
// Primary source: POST /accounts/{account_id}/access/apps/{app_id}/policies
func (c *HTTPClient) CreateAccessPolicy(ctx context.Context, accountID, appID string, emails []string) (AccessPolicy, error) {
	include := make([]map[string]any, 0, len(emails))
	for _, email := range emails {
		email = strings.TrimSpace(strings.ToLower(email))
		if email == "" {
			continue
		}
		include = append(include, map[string]any{"email": map[string]string{"email": email}})
	}
	if len(include) == 0 {
		return AccessPolicy{}, errors.New("access policy requires at least one email")
	}
	var result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	body := map[string]any{
		"name":       "Redgres allow",
		"decision":   "allow",
		"precedence": 1,
		"include":    include,
	}
	path := "/accounts/" + accountID + "/access/apps/" + appID + "/policies"
	if err := c.call(ctx, http.MethodPost, path, body, &result); err != nil {
		return AccessPolicy{}, err
	}
	if result.ID == "" {
		return AccessPolicy{}, errors.New("cloudflare access policy create returned empty id")
	}
	return AccessPolicy{ID: result.ID, Name: result.Name}, nil
}

// DeleteAccessApp deletes an Access application.
func (c *HTTPClient) DeleteAccessApp(ctx context.Context, accountID, appID string) error {
	var result any
	return c.call(ctx, http.MethodDelete, "/accounts/"+accountID+"/access/apps/"+appID, nil, &result)
}

// DeleteAccessPolicy deletes an Access application policy.
func (c *HTTPClient) DeleteAccessPolicy(ctx context.Context, accountID, appID, policyID string) error {
	var result any
	path := "/accounts/" + accountID + "/access/apps/" + appID + "/policies/" + policyID
	return c.call(ctx, http.MethodDelete, path, nil, &result)
}

// VerifyAccessApp checks an Access application still exists.
func (c *HTTPClient) VerifyAccessApp(ctx context.Context, accountID, appID string) error {
	var result struct {
		ID string `json:"id"`
	}
	return c.call(ctx, http.MethodGet, "/accounts/"+accountID+"/access/apps/"+appID, nil, &result)
}

// VerifyAccessPolicy checks an Access application policy still exists.
func (c *HTTPClient) VerifyAccessPolicy(ctx context.Context, accountID, appID, policyID string) error {
	var result struct {
		ID string `json:"id"`
	}
	path := "/accounts/" + accountID + "/access/apps/" + appID + "/policies/" + policyID
	return c.call(ctx, http.MethodGet, path, nil, &result)
}
