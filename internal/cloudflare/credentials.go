package cloudflare

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/SSujitX/redgres/internal/securefile"
)

// ReadOAuthClient loads OAuth client credentials from a 0600 JSON file.
func ReadOAuthClient(path string) (OAuthClientCredentials, error) {
	raw, err := securefile.ReadRegular(path, nil)
	if err != nil {
		return OAuthClientCredentials{}, err
	}
	var cred OAuthClientCredentials
	if err := json.Unmarshal(raw, &cred); err != nil {
		return OAuthClientCredentials{}, err
	}
	cred.ClientID = strings.TrimSpace(cred.ClientID)
	cred.ClientSecret = strings.TrimSpace(cred.ClientSecret)
	if cred.ClientID == "" || cred.ClientSecret == "" {
		return OAuthClientCredentials{}, os.ErrInvalid
	}
	return cred, nil
}

// WriteOAuthClient stores OAuth client credentials.
func WriteOAuthClient(path string, cred OAuthClientCredentials) error {
	raw, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	f, err := securefile.OpenRegular(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		return err
	}
	return f.Sync()
}

// ReadOAuthToken loads stored OAuth tokens.
func ReadOAuthToken(path string) (OAuthTokenSet, error) {
	raw, err := securefile.ReadRegular(path, nil)
	if err != nil {
		return OAuthTokenSet{}, err
	}
	var tok OAuthTokenSet
	if err := json.Unmarshal(raw, &tok); err != nil {
		return OAuthTokenSet{}, err
	}
	if tok.AccessToken == "" {
		return OAuthTokenSet{}, os.ErrInvalid
	}
	return tok, nil
}

// WriteOAuthToken stores OAuth tokens.
func WriteOAuthToken(path string, tok OAuthTokenSet) error {
	raw, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	f, err := securefile.OpenRegular(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		return err
	}
	return f.Sync()
}

// TokenExpired reports whether the access token should be refreshed.
func TokenExpired(tok OAuthTokenSet, now time.Time) bool {
	if tok.AccessToken == "" {
		return true
	}
	if tok.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(tok.ExpiresAt.Add(-30 * time.Second))
}
