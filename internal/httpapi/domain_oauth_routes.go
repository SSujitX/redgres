package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SSujitX/redgres/internal/cloudflare"
)

func (s *Server) handleDomainOAuthClient(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CloudflareOAuthClientFile == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "OAuth client file is not configured")
		return
	}
	var body struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	clientID := strings.TrimSpace(body.ClientID)
	clientSecret := strings.TrimSpace(body.ClientSecret)
	if clientID == "" || clientSecret == "" || len(clientID) > 256 || len(clientSecret) > 512 {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid OAuth client credentials")
		return
	}
	if err := cloudflare.WriteOAuthClient(s.cfg.CloudflareOAuthClientFile, cloudflare.OAuthClientCredentials{
		ClientID: clientID, ClientSecret: clientSecret,
	}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "OAuth client could not be stored")
		return
	}
	sess := sessionFrom(r)
	if err := s.audit.Record(sess.Username, "domain.oauth.client", "", "success", requestID(r), requestClientIP(r), map[string]any{"configured": true}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{"ok": true, "request_id": requestID(r)})
}

func (s *Server) handleDomainOAuthStart(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CloudflareOAuthClientFile == "" || s.cfg.CloudflareOAuthTokenFile == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "OAuth is not configured")
		return
	}
	store := domainStore{s.db}
	dep, err := store.Get(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "Configure domain hostnames before connecting OAuth")
		return
	}
	client, err := cloudflare.ReadOAuthClient(s.cfg.CloudflareOAuthClientFile)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "OAuth client is not configured")
		return
	}
	console := dep.consoleHostname()
	if console == "" {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "Console hostname is not configured")
		return
	}
	redirectURI := "https://" + console + "/api/v1/domain/oauth/callback"
	verifier, challenge, err := cloudflare.NewPKCEPair()
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Internal server error")
		return
	}
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Internal server error")
		return
	}
	state := hex.EncodeToString(stateBytes)
	sess := sessionFrom(r)
	if err := s.saveOAuthPending(r.Context(), sess.ID, hashOAuthState(state), verifier); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	authorizeURL, err := cloudflare.BuildAuthorizeURL(s.oauthEndpoints(), client.ClientID, redirectURI, state, challenge)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Internal server error")
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"authorize_url": authorizeURL,
		"redirect_uri":  redirectURI,
		"scopes":        cloudflare.RequiredOAuthScopes,
		"request_id":    requestID(r),
	})
}

func (s *Server) handleDomainOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid OAuth callback")
		return
	}
	sess := sessionFrom(r)
	if sess.ID == 0 {
		s.writeError(w, r, http.StatusUnauthorized, CodeUnauthorized, "Session required")
		return
	}
	expectedHash, verifier, err := s.loadOAuthPending(r.Context(), sess.ID)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "OAuth state is invalid or expired")
		return
	}
	if hashOAuthState(state) != expectedHash {
		s.clearOAuthPending(r.Context(), sess.ID)
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "OAuth state mismatch")
		return
	}
	store := domainStore{s.db}
	dep, err := store.Get(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusConflict, CodeConflict, "No domain configured")
		return
	}
	client, err := cloudflare.ReadOAuthClient(s.cfg.CloudflareOAuthClientFile)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "OAuth client is not configured")
		return
	}
	redirectURI := "https://" + dep.consoleHostname() + "/api/v1/domain/oauth/callback"
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	tokens, err := cloudflare.ExchangeAuthorizationCode(ctx, s.oauthEndpoints(), client.ClientID, client.ClientSecret, redirectURI, code, verifier)
	if err != nil {
		s.clearOAuthPending(ctx, sess.ID)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "OAuth token exchange failed")
		return
	}
	if err := s.audit.Record(sess.Username, "domain.oauth.connect", dep.consoleHostname(), "success", requestID(r), requestClientIP(r), map[string]any{"configured": true}); err != nil {
		s.clearOAuthPending(ctx, sess.ID)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	if err := cloudflare.WriteOAuthToken(s.cfg.CloudflareOAuthTokenFile, tokens); err != nil {
		s.clearOAuthPending(ctx, sess.ID)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "OAuth token could not be stored")
		return
	}
	s.clearOAuthPending(ctx, sess.ID)
	if s.cfg.CloudflareTokenFile != "" {
		_ = os.Remove(s.cfg.CloudflareTokenFile)
	}
	http.Redirect(w, r, "/?oauth=connected", http.StatusFound)
}
