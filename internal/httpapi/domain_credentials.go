package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/SSujitX/redgres/internal/cloudflare"
)

func (s *Server) oauthEndpoints() cloudflare.OAuthEndpoints {
	return cloudflare.OAuthEndpoints{
		AuthURL:   s.cfg.CloudflareOAuthAuthURL,
		TokenURL:  s.cfg.CloudflareOAuthTokenURL,
		RevokeURL: s.cfg.CloudflareOAuthRevokeURL,
	}
}

func (s *Server) resolveCloudflareBearer(ctx context.Context) (string, error) {
	if s.cfg.CloudflareOAuthTokenFile != "" && s.cfg.CloudflareOAuthClientFile != "" {
		tok, err := cloudflare.ReadOAuthToken(s.cfg.CloudflareOAuthTokenFile)
		if err == nil {
			if !cloudflare.TokenExpired(tok, time.Now().UTC()) {
				return tok.AccessToken, nil
			}
			client, err := cloudflare.ReadOAuthClient(s.cfg.CloudflareOAuthClientFile)
			if err != nil {
				return "", err
			}
			if tok.RefreshToken == "" {
				return "", errors.New("oauth token expired without refresh token")
			}
			refreshed, err := cloudflare.RefreshOAuthToken(ctx, s.oauthEndpoints(), client.ClientID, client.ClientSecret, tok.RefreshToken)
			if err != nil {
				return "", err
			}
			if refreshed.RefreshToken == "" {
				refreshed.RefreshToken = tok.RefreshToken
			}
			if err := cloudflare.WriteOAuthToken(s.cfg.CloudflareOAuthTokenFile, refreshed); err != nil {
				return "", err
			}
			return refreshed.AccessToken, nil
		}
	}
	if s.cfg.CloudflareTokenFile != "" {
		tok, err := readTokenFile(s.cfg.CloudflareTokenFile)
		if err == nil && tok != "" {
			return tok, nil
		}
	}
	return "", errors.New("cloudflare credentials are not configured")
}

func hashOAuthState(state string) string {
	sum := sha256.Sum256([]byte(state))
	return hex.EncodeToString(sum[:])
}

func (s *Server) saveOAuthPending(ctx context.Context, sessionID int64, stateHash, codeVerifier string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO domain_oauth_pending (session_id, state_hash, code_verifier, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET state_hash = excluded.state_hash, code_verifier = excluded.code_verifier, created_at = excluded.created_at`,
		sessionID, stateHash, codeVerifier, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

const oauthPendingMaxAge = 10 * time.Minute

func (s *Server) loadOAuthPending(ctx context.Context, sessionID int64) (stateHash, codeVerifier string, err error) {
	var createdAt string
	err = s.db.QueryRowContext(ctx,
		`SELECT state_hash, code_verifier, created_at FROM domain_oauth_pending WHERE session_id = ?`, sessionID).
		Scan(&stateHash, &codeVerifier, &createdAt)
	if err != nil {
		return "", "", err
	}
	if t, parseErr := time.Parse(time.RFC3339Nano, createdAt); parseErr != nil || time.Since(t) > oauthPendingMaxAge {
		s.clearOAuthPending(ctx, sessionID)
		return "", "", errors.New("oauth pending expired")
	}
	return stateHash, codeVerifier, nil
}

func (s *Server) clearOAuthPending(ctx context.Context, sessionID int64) {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM domain_oauth_pending WHERE session_id = ?`, sessionID)
}

func (s *Server) removeCloudflareCredentialFiles() {
	if s.cfg.CloudflareTokenFile != "" {
		_ = os.Remove(s.cfg.CloudflareTokenFile)
	}
	if s.cfg.CloudflareOAuthTokenFile != "" {
		_ = os.Remove(s.cfg.CloudflareOAuthTokenFile)
	}
	if s.cfg.CloudflareOAuthClientFile != "" {
		_ = os.Remove(s.cfg.CloudflareOAuthClientFile)
	}
	if s.cfg.CertbotDNSCredentialsFile != "" {
		_ = os.Remove(s.cfg.CertbotDNSCredentialsFile)
	}
	if s.cfg.TLSIssueRequestFile != "" {
		_ = os.Remove(s.cfg.TLSIssueRequestFile)
	}
	if s.cfg.TLSIssueResultFile != "" {
		_ = os.Remove(s.cfg.TLSIssueResultFile)
	}
}

func (s *Server) revokeOAuthIfPresent(ctx context.Context) {
	if s.cfg.CloudflareOAuthTokenFile == "" || s.cfg.CloudflareOAuthClientFile == "" {
		return
	}
	tok, err := cloudflare.ReadOAuthToken(s.cfg.CloudflareOAuthTokenFile)
	if err != nil {
		return
	}
	client, err := cloudflare.ReadOAuthClient(s.cfg.CloudflareOAuthClientFile)
	if err != nil {
		return
	}
	_ = cloudflare.RevokeOAuthToken(ctx, s.oauthEndpoints(), client.ClientID, client.ClientSecret, tok.RefreshToken)
}

func (s *Server) cloudflareClientFromConfig(ctx context.Context) (cloudflare.Client, error) {
	token, err := s.resolveCloudflareBearer(ctx)
	if err != nil {
		return nil, err
	}
	if s.cloudflare != nil {
		return s.cloudflare, nil
	}
	return &cloudflare.HTTPClient{Token: token}, nil
}
