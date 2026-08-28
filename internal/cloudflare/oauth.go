package cloudflare

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RequiredOAuthScopes are the minimal Cloudflare OAuth scopes for steady-state
// domain wizard mutations. Pin against GET /oauth/scopes before production sign-off.
var RequiredOAuthScopes = []string{
	"zone.read",
	"dns.write",
	"ssl-and-certificates.write",
	"user-details.read",
	"offline_access",
	"access:apps_and_policies:edit",
	"cloudflare_one.connectors:edit",
}

// OAuthClientCredentials is stored server-side at REDGRES_CLOUDFLARE_OAUTH_CLIENT_FILE.
type OAuthClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// OAuthTokenSet is stored server-side at REDGRES_CLOUDFLARE_OAUTH_TOKEN_FILE.
type OAuthTokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// OAuthEndpoints configures Cloudflare OAuth URLs.
type OAuthEndpoints struct {
	AuthURL    string
	TokenURL   string
	RevokeURL  string
	HTTPClient *http.Client
}

func (e OAuthEndpoints) client() *http.Client {
	if e.HTTPClient != nil {
		return e.HTTPClient
	}
	return http.DefaultClient
}

// NewPKCEPair returns a code_verifier and S256 code_challenge (base64url, no padding).
func NewPKCEPair() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// BuildAuthorizeURL constructs the Cloudflare OAuth authorization URL with PKCE.
func BuildAuthorizeURL(endpoints OAuthEndpoints, clientID, redirectURI, state, codeChallenge string) (string, error) {
	if clientID == "" || redirectURI == "" || state == "" || codeChallenge == "" {
		return "", errors.New("invalid oauth authorize parameters")
	}
	u, err := url.Parse(endpoints.AuthURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("scope", strings.Join(RequiredOAuthScopes, " "))
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ExchangeAuthorizationCode exchanges an authorization code for tokens.
func ExchangeAuthorizationCode(ctx context.Context, endpoints OAuthEndpoints, clientID, clientSecret, redirectURI, code, codeVerifier string) (OAuthTokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)
	return postToken(ctx, endpoints, clientID, clientSecret, form)
}

// RefreshOAuthToken refreshes an expired access token.
func RefreshOAuthToken(ctx context.Context, endpoints OAuthEndpoints, clientID, clientSecret, refreshToken string) (OAuthTokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	return postToken(ctx, endpoints, clientID, clientSecret, form)
}

// RevokeOAuthToken revokes a refresh or access token.
func RevokeOAuthToken(ctx context.Context, endpoints OAuthEndpoints, clientID, clientSecret, token string) error {
	if token == "" {
		return nil
	}
	form := url.Values{}
	form.Set("token", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.RevokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)
	resp, err := endpoints.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("oauth revoke failed: %d", resp.StatusCode)
	}
	return nil
}

func postToken(ctx context.Context, endpoints OAuthEndpoints, clientID, clientSecret string, form url.Values) (OAuthTokenSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return OAuthTokenSet{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)
	resp, err := endpoints.client().Do(req)
	if err != nil {
		return OAuthTokenSet{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return OAuthTokenSet{}, err
	}
	if resp.StatusCode >= 400 {
		return OAuthTokenSet{}, fmt.Errorf("oauth token exchange failed: %d", resp.StatusCode)
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return OAuthTokenSet{}, err
	}
	if raw.AccessToken == "" {
		return OAuthTokenSet{}, errors.New("oauth token response missing access_token")
	}
	expires := time.Now().UTC().Add(time.Duration(raw.ExpiresIn) * time.Second)
	if raw.ExpiresIn <= 0 {
		expires = time.Now().UTC().Add(30 * time.Minute)
	}
	return OAuthTokenSet{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAt:    expires,
	}, nil
}
