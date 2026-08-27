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
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// BaseURL is the Cloudflare v4 API root.
const BaseURL = "https://api.cloudflare.com/client/v4"

// ErrNotFound is returned when Cloudflare reports the resource does not exist.
var ErrNotFound = errors.New("cloudflare resource not found")

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
	ID   string
	Name string
}

// AccessApp is a created Cloudflare Access application.
type AccessApp struct {
	ID     string
	Domain string
}

// Client is the Cloudflare operations the wizard needs. A fake implements this
// for tests; HTTPClient implements it against the live API.
type Client interface {
	DiscoverZone(ctx context.Context, name string) (Zone, error)
	CreateTunnel(ctx context.Context, accountID, name, secret string) (Tunnel, error)
	CreateRecord(ctx context.Context, zoneID, name, content string, proxied bool) (Record, error)
	CreateAccessApp(ctx context.Context, accountID, domain string) (AccessApp, error)
	DeleteTunnel(ctx context.Context, accountID, tunnelID string) error
	DeleteRecord(ctx context.Context, zoneID, recordID string) error
	DeleteAccessApp(ctx context.Context, accountID, appID string) error
	VerifyTunnel(ctx context.Context, accountID, tunnelID string) error
	VerifyRecord(ctx context.Context, zoneID, recordID string) error
	VerifyAccessApp(ctx context.Context, accountID, appID string) error
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
	if err := c.call(ctx, http.MethodGet, "/zones?name="+name, nil, &result); err != nil {
		return Zone{}, err
	}
	for _, z := range result {
		if strings.EqualFold(z.Name, name) {
			return Zone{ID: z.ID, Name: z.Name, AccountID: z.Acct.ID}, nil
		}
	}
	return Zone{}, ErrNotFound
}

// CreateTunnel creates a remotely-managed tunnel.
func (c *HTTPClient) CreateTunnel(ctx context.Context, accountID, name, secret string) (Tunnel, error) {
	var result struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	body := map[string]string{"name": name, "tunnel_secret": secret}
	if err := c.call(ctx, http.MethodPost, "/accounts/"+accountID+"/cfd_tunnel", body, &result); err != nil {
		return Tunnel{}, err
	}
	return Tunnel{ID: result.ID, Name: result.Name, Token: result.Token}, nil
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

// CreateRecord creates a DNS record.
func (c *HTTPClient) CreateRecord(ctx context.Context, zoneID, name, content string, proxied bool) (Record, error) {
	var result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	body := map[string]any{
		"type":    "CNAME",
		"name":    name,
		"content": content,
		"proxied": proxied,
		"ttl":     1,
	}
	if err := c.call(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", body, &result); err != nil {
		return Record{}, err
	}
	return Record{ID: result.ID, Name: result.Name}, nil
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

// DeleteAccessApp deletes an Access application.
func (c *HTTPClient) DeleteAccessApp(ctx context.Context, accountID, appID string) error {
	var result any
	return c.call(ctx, http.MethodDelete, "/accounts/"+accountID+"/access/apps/"+appID, nil, &result)
}

// VerifyAccessApp checks an Access application still exists.
func (c *HTTPClient) VerifyAccessApp(ctx context.Context, accountID, appID string) error {
	var result struct {
		ID string `json:"id"`
	}
	return c.call(ctx, http.MethodGet, "/accounts/"+accountID+"/access/apps/"+appID, nil, &result)
}

// SecretForTunnel generates a base64-encoded 32-byte tunnel secret.
func SecretForTunnel() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}
