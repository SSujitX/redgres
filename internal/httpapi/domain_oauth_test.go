package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/cloudflare"
	"github.com/SSujitX/redgres/internal/tlsops"
)

func TestDomainOAuthFlow(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	srv.cloudflare = newFakeCF()
	srv.cfg.CloudflareOAuthClientFile = filepath.Join(t.TempDir(), "oauth-client.json")
	srv.cfg.CloudflareOAuthTokenFile = filepath.Join(t.TempDir(), "oauth-token.json")

	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, domainApplyBody)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}

	var tokenCalls int
	oauthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			tokenCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "oauth-access-canary",
				"refresh_token": "oauth-refresh-canary",
				"expires_in":    3600,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer oauthSrv.Close()
	srv.cfg.CloudflareOAuthAuthURL = oauthSrv.URL + "/oauth2/auth"
	srv.cfg.CloudflareOAuthTokenURL = oauthSrv.URL + "/oauth2/token"
	srv.cfg.CloudflareOAuthRevokeURL = oauthSrv.URL + "/oauth2/revoke"

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/oauth-client", cookie, csrf, `{"client_id":"cid","client_secret":"csecret"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("oauth client = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "csecret") {
		t.Fatal("client secret leaked")
	}

	rec = serve(handler, authed(http.MethodPost, "/api/v1/domain/oauth/start", cookie, csrf, `{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("oauth start = %d %s", rec.Code, rec.Body.String())
	}

	rec = serve(handler, authed(http.MethodGet, "/api/v1/domain/oauth/callback?code=abc&state=wrong", cookie, csrf, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad state = %d %s", rec.Code, rec.Body.String())
	}

	rec = serve(handler, authed(http.MethodPost, "/api/v1/domain/oauth/start", cookie, csrf, `{}`))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var startBody struct {
		AuthorizeURL string `json:"authorize_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &startBody); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(startBody.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	state := u.Query().Get("state")
	rec = serve(handler, authed(http.MethodGet, "/api/v1/domain/oauth/callback?code=good&state="+state, cookie, csrf, ""))
	if rec.Code != http.StatusFound {
		t.Fatalf("callback = %d %s", rec.Code, rec.Body.String())
	}
	if tokenCalls == 0 {
		t.Fatal("token exchange was not called")
	}
	if _, err := os.Stat(srv.cfg.CloudflareOAuthTokenFile); err != nil {
		t.Fatal("oauth token file missing")
	}
	for _, row := range auditRows(t, srv) {
		if strings.Contains(row, "oauth-access-canary") || strings.Contains(row, "oauth-refresh-canary") {
			t.Fatalf("oauth token leaked in audit: %s", row)
		}
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatal("api token file should be removed after oauth connect")
	}
}

func TestDomainAccessPolicyCompensatesOnVerifyFailure(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	fake := newFakeCF()
	srv.cloudflare = fake
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, domainApplyBody)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	fake.verifyErr = cloudflare.ErrNotFound
	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/access-policy", cookie, csrf, `{"emails":["owner@example.com"]}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("access policy = %d %s", rec.Code, rec.Body.String())
	}
	if len(fake.deletedP) != 1 {
		t.Fatalf("expected policy compensation delete: %v", fake.deletedP)
	}
}

func TestDomainManualWizardCompletion(t *testing.T) {
	srv, handler, cookie, csrf, _ := newDomainServer(t)
	body := `{"dns_provider":"manual","zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"console.example.com","db":"db.example.com","redis":"redis.example.com"}}`
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, body)); rec.Code != http.StatusOK {
		t.Fatalf("manual apply = %d %s", rec.Code, rec.Body.String())
	}
	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/confirm-reachable", cookie, csrf, `{}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("confirm before access = %d %s", rec.Code, rec.Body.String())
	}
	rec = serve(handler, authed(http.MethodPost, "/api/v1/domain/manual/confirm-access", cookie, csrf, `{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("manual confirm access = %d %s", rec.Code, rec.Body.String())
	}
	boot := &fakeBootstrap{open: true}
	srv.SetBootstrapCloser(boot)
	rec = serve(handler, authed(http.MethodPost, "/api/v1/domain/confirm-reachable", cookie, csrf, `{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"bootstrap_closed":true`) {
		t.Fatalf("expected bootstrap closed: %s", rec.Body.String())
	}
}

func TestDomainManualApplyWithoutCloudflareConfig(t *testing.T) {
	_, handler, cookie, csrf, _ := newDomainServer(t)
	body := `{"dns_provider":"manual","zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"console.example.com","db":"db.example.com","redis":"redis.example.com"}}`
	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("manual apply without cf config = %d %s", rec.Code, rec.Body.String())
	}
}

func TestDomainManualGetReturnsInstructions(t *testing.T) {
	_, handler, cookie, csrf, _ := newDomainServer(t)
	body := `{"dns_provider":"manual","zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"console.example.com","db":"db.example.com","redis":"redis.example.com"}}`
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, body)); rec.Code != http.StatusOK {
		t.Fatalf("manual apply = %d %s", rec.Code, rec.Body.String())
	}
	rec := serve(handler, authed(http.MethodGet, "/api/v1/domain", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("domain get = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"instructions"`) {
		t.Fatalf("expected instructions on GET: %s", rec.Body.String())
	}
}

func TestDomainManualApplyReturnsInstructions(t *testing.T) {
	_, handler, cookie, csrf, _ := newDomainServer(t)
	body := `{"dns_provider":"manual","zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"console.example.com","db":"db.example.com","redis":"redis.example.com"}}`
	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("manual apply = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"instructions"`) {
		t.Fatalf("expected instructions: %s", rec.Body.String())
	}
}

func TestDomainOAuthPendingRejectsCorruptCreatedAt(t *testing.T) {
	srv, handler, cookie, csrf, _ := newDomainServer(t)
	srv.cfg.CloudflareOAuthClientFile = filepath.Join(t.TempDir(), "oauth-client.json")
	srv.cfg.CloudflareOAuthTokenFile = filepath.Join(t.TempDir(), "oauth-token.json")
	srv.cfg.CloudflareOAuthAuthURL = "https://example.com/oauth2/auth"
	srv.cfg.CloudflareOAuthTokenURL = "https://example.com/oauth2/token"
	body := `{"dns_provider":"manual","zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"console.example.com","db":"db.example.com","redis":"redis.example.com"}}`
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, body)); rec.Code != http.StatusOK {
		t.Fatalf("manual apply = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/oauth-client", cookie, csrf, `{"client_id":"cid","client_secret":"csecret"}`)); rec.Code != http.StatusOK {
		t.Fatalf("oauth client = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/oauth/start", cookie, csrf, `{}`)); rec.Code != http.StatusOK {
		t.Fatalf("oauth start = %d %s", rec.Code, rec.Body.String())
	}
	var sessionID int64
	if err := srv.db.QueryRow(`SELECT session_id FROM domain_oauth_pending LIMIT 1`).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.db.Exec(`UPDATE domain_oauth_pending SET created_at = ? WHERE session_id = ?`, "not-a-timestamp", sessionID); err != nil {
		t.Fatal(err)
	}
	rec := serve(handler, authed(http.MethodGet, "/api/v1/domain/oauth/callback?code=good&state=anything", cookie, csrf, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("corrupt created_at = %d %s", rec.Code, rec.Body.String())
	}
	var pendingCount int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM domain_oauth_pending WHERE session_id = ?`, sessionID).Scan(&pendingCount); err != nil {
		t.Fatal(err)
	}
	if pendingCount != 0 {
		t.Fatal("expected pending row cleared after corrupt created_at")
	}
}

func TestDomainTLSIssueWithFakeCertbot(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	srv.cloudflare = newFakeCF()
	tmp := t.TempDir()
	creds := filepath.Join(tmp, "dns.ini")
	if err := os.WriteFile(creds, []byte("dns_cloudflare_api_token = x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv.cfg.CertbotDNSCredentialsFile = creds
	live := filepath.Join(tmp, "live")
	prevLive := tlsops.CertLiveDir
	tlsops.CertLiveDir = live
	t.Cleanup(func() { tlsops.CertLiveDir = prevLive })

	writeTestLECert(t, live, "db.example.com", "rs.example.com")

	fakeBin := filepath.Join(tmp, "certbot.bat")
	if err := os.WriteFile(fakeBin, []byte("@echo off\nexit /b 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv.cfg.CertbotBin = fakeBin

	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, domainApplyBody)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/tls/issue", cookie, csrf, `{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("tls issue = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"issued"`) {
		t.Fatalf("expected issued tls: %s", rec.Body.String())
	}
}

func writeTestLECert(t *testing.T, liveRoot, primary string, sans ...string) {
	t.Helper()
	if len(sans) == 0 {
		sans = []string{primary}
	} else {
		found := false
		for _, name := range sans {
			if name == primary {
				found = true
				break
			}
		}
		if !found {
			sans = append([]string{primary}, sans...)
		}
	}
	dir := filepath.Join(liveRoot, primary)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: primary},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     sans,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "fullchain.pem"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatal(err)
	}
}
