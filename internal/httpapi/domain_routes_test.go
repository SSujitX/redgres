package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/bootstrap"
	"github.com/SSujitX/redgres/internal/cloudflare"
	"github.com/SSujitX/redgres/internal/config"
)

type fakeCF struct {
	zone              cloudflare.Zone
	tunnels           map[string]cloudflare.Tunnel
	records           map[string]cloudflare.Record
	apps              map[string]cloudflare.AccessApp
	policies          map[string]cloudflare.AccessPolicy
	deletedT          []string
	deletedR          []string
	deletedA          []string
	deletedP          []string
	verifyErr         error
	createTunnelErr   error
	lastIngressHost   string
	lastIngressOrigin string
	lastIngressRoutes []cloudflare.IngressRoute
	lastPolicyEmails  []string
	nextRecID         int
	nextAppID         int
}

func newFakeCF() *fakeCF {
	return &fakeCF{
		zone:     cloudflare.Zone{ID: "zone-1", Name: "example.com", AccountID: "acct-1"},
		tunnels:  map[string]cloudflare.Tunnel{},
		records:  map[string]cloudflare.Record{},
		apps:     map[string]cloudflare.AccessApp{},
		policies: map[string]cloudflare.AccessPolicy{},
	}
}

func (f *fakeCF) DiscoverZone(_ context.Context, name string) (cloudflare.Zone, error) {
	if name != "example.com" {
		return cloudflare.Zone{}, cloudflare.ErrNotFound
	}
	return f.zone, nil
}

func (f *fakeCF) CreateTunnel(_ context.Context, accountID, name string) (cloudflare.Tunnel, error) {
	if f.createTunnelErr != nil {
		return cloudflare.Tunnel{}, f.createTunnelErr
	}
	t := cloudflare.Tunnel{ID: "tun-1", Name: name, Token: "fake-tunnel-token"}
	f.tunnels[t.ID] = t
	return t, nil
}

func (f *fakeCF) ConfigureIngress(_ context.Context, accountID, tunnelID, hostname, originHTTP string) error {
	return f.ConfigureIngressRoutes(context.Background(), accountID, tunnelID, []cloudflare.IngressRoute{{Hostname: hostname, Service: originHTTP}})
}

func (f *fakeCF) ConfigureIngressRoutes(_ context.Context, accountID, tunnelID string, routes []cloudflare.IngressRoute) error {
	if _, ok := f.tunnels[tunnelID]; !ok {
		return cloudflare.ErrNotFound
	}
	f.lastIngressRoutes = append([]cloudflare.IngressRoute{}, routes...)
	if len(routes) > 0 {
		f.lastIngressHost = routes[0].Hostname
		f.lastIngressOrigin = routes[0].Service
	}
	return nil
}

func (f *fakeCF) CreateRecord(_ context.Context, zoneID, name, content string, proxied bool) (cloudflare.Record, error) {
	return f.CreateDNSRecord(context.Background(), zoneID, name, "CNAME", content, proxied)
}

func (f *fakeCF) CreateDNSRecord(_ context.Context, zoneID, name, recordType, content string, proxied bool) (cloudflare.Record, error) {
	f.nextRecID++
	id := fmt.Sprintf("rec-%d", f.nextRecID)
	r := cloudflare.Record{ID: id, Name: name, Type: recordType, Proxied: proxied}
	f.records[r.ID] = r
	return r, nil
}

func (f *fakeCF) CreateAccessApp(_ context.Context, accountID, domain string) (cloudflare.AccessApp, error) {
	f.nextAppID++
	id := fmt.Sprintf("app-%d", f.nextAppID)
	a := cloudflare.AccessApp{ID: id, Domain: domain}
	f.apps[a.ID] = a
	return a, nil
}

func (f *fakeCF) CreateAccessPolicy(_ context.Context, accountID, appID string, emails []string) (cloudflare.AccessPolicy, error) {
	if _, ok := f.apps[appID]; !ok {
		return cloudflare.AccessPolicy{}, cloudflare.ErrNotFound
	}
	f.lastPolicyEmails = append([]string{}, emails...)
	p := cloudflare.AccessPolicy{ID: "pol-1", Name: "Redgres allow"}
	f.policies[p.ID] = p
	return p, nil
}

func (f *fakeCF) DeleteTunnel(_ context.Context, accountID, id string) error {
	f.deletedT = append(f.deletedT, id)
	delete(f.tunnels, id)
	return nil
}

func (f *fakeCF) DeleteRecord(_ context.Context, zoneID, id string) error {
	f.deletedR = append(f.deletedR, id)
	delete(f.records, id)
	return nil
}

func (f *fakeCF) DeleteAccessApp(_ context.Context, accountID, id string) error {
	f.deletedA = append(f.deletedA, id)
	delete(f.apps, id)
	return nil
}

func (f *fakeCF) DeleteAccessPolicy(_ context.Context, accountID, appID, policyID string) error {
	f.deletedP = append(f.deletedP, policyID)
	delete(f.policies, policyID)
	return nil
}

func (f *fakeCF) VerifyTunnel(_ context.Context, accountID, id string) error {
	if f.verifyErr != nil {
		return f.verifyErr
	}
	if _, ok := f.tunnels[id]; !ok {
		return cloudflare.ErrNotFound
	}
	return nil
}

func (f *fakeCF) VerifyRecord(_ context.Context, zoneID, id string) error {
	if f.verifyErr != nil {
		return f.verifyErr
	}
	if _, ok := f.records[id]; !ok {
		return cloudflare.ErrNotFound
	}
	return nil
}

func (f *fakeCF) VerifyAccessApp(_ context.Context, accountID, id string) error {
	if f.verifyErr != nil {
		return f.verifyErr
	}
	if _, ok := f.apps[id]; !ok {
		return cloudflare.ErrNotFound
	}
	return nil
}

func (f *fakeCF) VerifyAccessPolicy(_ context.Context, accountID, appID, policyID string) error {
	if f.verifyErr != nil {
		return f.verifyErr
	}
	if _, ok := f.apps[appID]; !ok {
		return cloudflare.ErrNotFound
	}
	if _, ok := f.policies[policyID]; !ok {
		return cloudflare.ErrNotFound
	}
	return nil
}

func newDomainServer(t *testing.T) (*Server, http.Handler, string, string, string) {
	t.Helper()
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	handler := srv.Handler()
	cookie, csrf := login(t, handler)
	tokenPath := filepath.Join(t.TempDir(), "cloudflare-token")
	srv.cfg.CloudflareTokenFile = tokenPath
	srv.cfg.TunnelTokenFile = filepath.Join(t.TempDir(), "tunnel-token")
	srv.cfg.Address = "127.0.0.1:8790"
	return srv, handler, cookie, csrf, tokenPath
}

const domainApplyBody = `{"zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"console.example.com","db":"db.example.com","rs":"rs.example.com","pgadmin":"pgadmin.example.com","redis":"redis.example.com"}}`

func auditRows(t *testing.T, srv *Server) []string {
	t.Helper()
	rows, err := srv.db.Query(`SELECT action, outcome, metadata FROM audit_events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var action, outcome, metadata string
		if err := rows.Scan(&action, &outcome, &metadata); err != nil {
			t.Fatal(err)
		}
		out = append(out, action+"|"+outcome+"|"+metadata)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDomainCapabilityPresent(t *testing.T) {
	if !hasCapability("platform.network") {
		t.Fatal("platform.network must be a default capability")
	}
}

func TestDomainTokenSetStoresWithoutLeak(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	const canary = "canary-token-abc123"

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/token", cookie, csrf, `{"token":"`+canary+`"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("token set = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), canary) {
		t.Fatalf("token leaked in response: %s", rec.Body.String())
	}
	raw, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != canary {
		t.Fatalf("token file = %q", string(raw))
	}
	for _, row := range auditRows(t, srv) {
		if strings.Contains(row, canary) {
			t.Fatalf("token leaked in audit: %s", row)
		}
	}
}

func TestDomainApplyCreatesPersistsAndDoesNotAutoClose(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	fake := newFakeCF()
	srv.cloudflare = fake
	boot := &fakeBootstrap{open: true}
	srv.SetBootstrapCloser(boot)

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, domainApplyBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"bootstrap_still_open":true`) {
		t.Fatalf("expected bootstrap_still_open=true: %s", rec.Body.String())
	}
	if !boot.open || boot.closed != 0 {
		t.Fatalf("apply closed bootstrap: open=%v closed=%d", boot.open, boot.closed)
	}
	if strings.Contains(rec.Body.String(), "fake-tunnel-token") {
		t.Fatal("tunnel token leaked in response")
	}
	raw, err := os.ReadFile(srv.cfg.TunnelTokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "fake-tunnel-token" {
		t.Fatalf("tunnel token file = %q", string(raw))
	}
	if _, ok := fake.tunnels["tun-1"]; !ok {
		t.Fatal("tunnel was not created")
	}
	if _, ok := fake.records["rec-1"]; !ok {
		t.Fatal("record was not created")
	}
	if _, ok := fake.apps["app-1"]; !ok {
		t.Fatal("access app was not created")
	}
	if len(fake.lastIngressRoutes) != 3 {
		t.Fatalf("ingress routes = %d", len(fake.lastIngressRoutes))
	}
	if fake.lastIngressHost != "console.example.com" || fake.lastIngressOrigin != "http://127.0.0.1:8790" {
		t.Fatalf("ingress host=%q origin=%q", fake.lastIngressHost, fake.lastIngressOrigin)
	}
	if srv.cfg.PgAdminURL != "https://pgadmin.example.com" || srv.cfg.RedisInsightURL != "https://redis.example.com" {
		t.Fatalf("tool urls = %q %q", srv.cfg.PgAdminURL, srv.cfg.RedisInsightURL)
	}
	if srv.ConsoleOrigin().Get() != "https://console.example.com" {
		t.Fatalf("console origin = %q", srv.ConsoleOrigin().Get())
	}

	rec = serve(handler, authed(http.MethodGet, "/api/v1/domain", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("domain get = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"configured":true`) {
		t.Fatalf("expected configured=true: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"tunnel_id":"tun-1"`) {
		t.Fatalf("expected tunnel_id on GET: %s", rec.Body.String())
	}
}

func TestDomainApplyLogsWithoutSecrets(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	srv.log = slog.New(slog.NewTextHandler(&logs, nil))
	srv.cloudflare = newFakeCF()
	srv.SetBootstrapCloser(&fakeBootstrap{open: true})

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, domainApplyBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	out := logs.String()
	if !strings.Contains(out, "domain.apply") || !strings.Contains(out, "example.com") || !strings.Contains(out, "console.example.com") {
		t.Fatalf("expected secret-safe apply log: %s", out)
	}
	for _, leak := range []string{"fake-tunnel-token", "test-token", "eyJ"} {
		if strings.Contains(out, leak) {
			t.Fatalf("apply log leaked %q: %s", leak, out)
		}
	}
	for _, row := range auditRows(t, srv) {
		if !strings.Contains(row, "domain.apply") {
			continue
		}
		if !strings.Contains(row, `"zone":"example.com"`) {
			t.Fatalf("apply audit missing zone: %s", row)
		}
		if strings.Contains(row, "fake-tunnel-token") || strings.Contains(row, "test-token") {
			t.Fatalf("apply audit leaked secret: %s", row)
		}
	}
}

func TestDomainApplyWritesCertbotDNSCredentialsFromAPIToken(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	const apiToken = "canary-apply-api-token"
	if err := writeTokenFile(tokenPath, apiToken); err != nil {
		t.Fatal(err)
	}
	creds := filepath.Join(t.TempDir(), "certbot-dns.ini")
	request := filepath.Join(t.TempDir(), "tls-issue.request")
	srv.cfg.CertbotDNSCredentialsFile = creds
	srv.cfg.TLSIssueRequestFile = request
	srv.cloudflare = newFakeCF()
	srv.SetBootstrapCloser(&fakeBootstrap{open: true})

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, domainApplyBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), apiToken) {
		t.Fatal("API token leaked in apply response")
	}
	raw, err := os.ReadFile(creds)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "dns_cloudflare_api_token = "+apiToken+"\n" {
		t.Fatalf("certbot ini = %q", raw)
	}
	reqRaw, err := os.ReadFile(request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(reqRaw)) != "db.example.com\nrs.example.com" {
		t.Fatalf("tls request = %q", reqRaw)
	}
	if strings.Contains(string(reqRaw), apiToken) {
		t.Fatal("API token leaked in tls request")
	}
	for _, row := range auditRows(t, srv) {
		if strings.Contains(row, apiToken) {
			t.Fatalf("apply leaked token in audit: %s", row)
		}
	}

	rec = serve(handler, authed(http.MethodDelete, "/api/v1/domain", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("disconnect = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(creds); !os.IsNotExist(err) {
		t.Fatal("certbot ini left after disconnect")
	}
	if _, err := os.Stat(request); !os.IsNotExist(err) {
		t.Fatal("tls request left after disconnect")
	}
	if strings.Contains(rec.Body.String(), apiToken) {
		t.Fatal("API token leaked in disconnect response")
	}
}

func TestDomainDisconnectDeletesOnlyCreated(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	fake := newFakeCF()
	srv.cloudflare = fake

	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, domainApplyBody)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}

	rec := serve(handler, authed(http.MethodDelete, "/api/v1/domain", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("disconnect = %d %s", rec.Code, rec.Body.String())
	}
	if len(fake.deletedA) != 3 {
		t.Fatalf("deleted apps = %v", fake.deletedA)
	}
	if len(fake.deletedR) != 5 {
		t.Fatalf("deleted records = %v", fake.deletedR)
	}
	if len(fake.deletedT) != 1 || fake.deletedT[0] != "tun-1" {
		t.Fatalf("deleted tunnels = %v", fake.deletedT)
	}

	rec = serve(handler, authed(http.MethodGet, "/api/v1/domain", cookie, csrf, ""))
	if !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Fatalf("expected configured=false after disconnect: %s", rec.Body.String())
	}
}

func TestDomainApplyRequiresToken(t *testing.T) {
	srv, handler, cookie, csrf, _ := newDomainServer(t)
	fake := newFakeCF()
	srv.cloudflare = fake

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, domainApplyBody))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("apply without token = %d %s", rec.Code, rec.Body.String())
	}
}

func TestDomainApplyConflictsOnReapply(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	fake := newFakeCF()
	srv.cloudflare = fake

	body := domainApplyBody
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, body)); rec.Code != http.StatusOK {
		t.Fatalf("first apply = %d %s", rec.Code, rec.Body.String())
	}
	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("reapply = %d %s", rec.Code, rec.Body.String())
	}
}

func TestDomainApplyConflictWhenTunnelNameInUse(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	fake := newFakeCF()
	fake.createTunnelErr = cloudflare.ErrTunnelNameInUse
	srv.cloudflare = fake

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, domainApplyBody))
	if rec.Code != http.StatusConflict {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"conflict"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already exists") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestDomainApplyCompensatesOnVerifyFailure(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	fake := newFakeCF()
	fake.verifyErr = cloudflare.ErrNotFound
	srv.cloudflare = fake

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, domainApplyBody))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if len(fake.deletedA) != 3 {
		t.Fatalf("compensation did not delete access apps: %v", fake.deletedA)
	}
	if len(fake.deletedR) != 5 {
		t.Fatalf("compensation did not delete records: %v", fake.deletedR)
	}
	if len(fake.deletedT) != 1 || fake.deletedT[0] != "tun-1" {
		t.Fatalf("compensation did not delete tunnel: %v", fake.deletedT)
	}
}

type fakeBootstrap struct {
	open   bool
	closed int
}

func (f *fakeBootstrap) Open() bool { return f.open }
func (f *fakeBootstrap) Close() error {
	f.open = false
	f.closed++
	return nil
}
func (f *fakeBootstrap) Shutdown(context.Context) error { return f.Close() }

func TestDomainAccessPolicyAndConfirmReachable(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	fake := newFakeCF()
	srv.cloudflare = fake
	boot := &fakeBootstrap{open: true}
	srv.SetBootstrapCloser(boot)

	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, `{"zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"redgres.example.com","db":"db.example.com","rs":"rs.example.com","pgadmin":"pgadmin.example.com","redis":"redis.example.com"}}`)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/confirm-reachable", cookie, csrf, `{}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("confirm before allow = %d %s", rec.Code, rec.Body.String())
	}

	rec = serve(handler, authed(http.MethodPost, "/api/v1/domain/access-policy", cookie, csrf, `{"emails":["Owner@Example.COM","owner@example.com"]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("access policy = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"access":"allow"`) {
		t.Fatalf("expected access=allow: %s", rec.Body.String())
	}
	if len(fake.lastPolicyEmails) != 1 || fake.lastPolicyEmails[0] != "owner@example.com" {
		t.Fatalf("policy emails = %#v", fake.lastPolicyEmails)
	}
	if strings.Contains(rec.Body.String(), "owner@example.com") {
		t.Fatal("email leaked in access-policy response")
	}

	rec = serve(handler, authed(http.MethodGet, "/api/v1/domain", cookie, csrf, ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"access":"allow"`) || !strings.Contains(rec.Body.String(), `"bootstrap_still_open":true`) {
		t.Fatalf("get after allow = %d %s", rec.Code, rec.Body.String())
	}

	rec = serve(handler, authed(http.MethodPost, "/api/v1/domain/access-policy", cookie, csrf, `{"emails":["other@example.com"]}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-allow = %d %s", rec.Code, rec.Body.String())
	}

	rec = serve(handler, authed(http.MethodPost, "/api/v1/domain/confirm-reachable", cookie, csrf, `{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"bootstrap_still_open":false`) || !strings.Contains(rec.Body.String(), `"bootstrap_closed":true`) {
		t.Fatalf("confirm body = %s", rec.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && boot.closed == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if boot.open || boot.closed != 1 {
		t.Fatalf("bootstrap open=%v closed=%d", boot.open, boot.closed)
	}

	rec = serve(handler, authed(http.MethodDelete, "/api/v1/domain", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("disconnect = %d %s", rec.Code, rec.Body.String())
	}
	if len(fake.deletedP) != 3 || fake.deletedP[0] != "pol-1" {
		t.Fatalf("disconnect did not delete policies: %v", fake.deletedP)
	}
}

func TestDomainAccessPolicyRejectsBadEmail(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	srv.cloudflare = newFakeCF()
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, `{"zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"redgres.example.com","db":"db.example.com","rs":"rs.example.com","pgadmin":"pgadmin.example.com","redis":"redis.example.com"}}`)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/access-policy", cookie, csrf, `{"emails":["not-an-email"]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad email = %d %s", rec.Code, rec.Body.String())
	}
}

func TestDomainConfirmReachableReturnsBodyOnBootstrapListener(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("net/http.Server.Shutdown races the client read on Windows")
	}
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	srv.cloudflare = newFakeCF()

	boot := bootstrap.New(handler, "127.0.0.1:0", time.Minute)
	if err := boot.Start(); err != nil {
		t.Fatalf("bootstrap Start: %v", err)
	}
	defer boot.Close()
	srv.SetBootstrapCloser(boot)
	envFile := filepath.Join(t.TempDir(), "redgres.env")
	if err := os.WriteFile(envFile, []byte("REDGRES_BOOTSTRAP_ADDRESS=0.0.0.0:8989\nREDGRES_COOKIE_SECURE=false\nREDGRES_BASE_URL=http://127.0.0.1:8790\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	srv.cfg.EnvFile = envFile

	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, `{"zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"redgres.example.com","db":"db.example.com","rs":"rs.example.com","pgadmin":"pgadmin.example.com","redis":"redis.example.com"}}`)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/access-policy", cookie, csrf, `{"emails":["owner@example.com"]}`)); rec.Code != http.StatusOK {
		t.Fatalf("access policy = %d %s", rec.Code, rec.Body.String())
	}

	url := "http://" + boot.Addr() + "/api/v1/domain/confirm-reachable"
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	req.Header.Set(csrfHeader, csrf)
	req.Header.Set("Origin", "http://127.0.0.1:8790")
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("confirm over bootstrap: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm status = %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"ok":true`) || !strings.Contains(string(body), `"bootstrap_closed":true`) {
		t.Fatalf("confirm body incomplete: %s", body)
	}

	addr := boot.Addr()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return
		}
		conn.Close()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("bootstrap %s still accepting after confirm", addr)
}

func TestDomainConfirmReachablePersistsClosedState(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	srv.cloudflare = newFakeCF()
	boot := &fakeBootstrap{open: true}
	srv.SetBootstrapCloser(boot)

	envFile := filepath.Join(t.TempDir(), "redgres.env")
	if err := os.WriteFile(envFile, []byte("REDGRES_BOOTSTRAP_ADDRESS=0.0.0.0:8989\nREDGRES_COOKIE_SECURE=false\nREDGRES_BASE_URL=http://203.0.113.10:8989\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	srv.cfg.EnvFile = envFile

	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, `{"zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"redgres.example.com","db":"db.example.com","rs":"rs.example.com","pgadmin":"pgadmin.example.com","redis":"redis.example.com"}}`)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/access-policy", cookie, csrf, `{"emails":["owner@example.com"]}`)); rec.Code != http.StatusOK {
		t.Fatalf("access policy = %d %s", rec.Code, rec.Body.String())
	}
	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/confirm-reachable", cookie, csrf, `{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d %s", rec.Code, rec.Body.String())
	}
	if !bootstrap.MarkerPresent(srv.cfg.SQLitePath) {
		t.Fatal("confirm did not persist bootstrap.closed")
	}
	got, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "0.0.0.0:8989") || strings.Contains(text, "COOKIE_SECURE=false") {
		t.Fatalf("env still in bootstrap window: %s", text)
	}
	if !strings.Contains(text, "REDGRES_BASE_URL=https://redgres.example.com") {
		t.Fatalf("env base URL: %s", text)
	}
	if srv.cfg.BaseURL != "https://redgres.example.com" || !srv.cfg.CookieSecure || srv.cfg.BootstrapAddress != "" {
		t.Fatalf("in-memory cfg not hot-updated: base=%q secure=%v boot=%q", srv.cfg.BaseURL, srv.cfg.CookieSecure, srv.cfg.BootstrapAddress)
	}
}

func TestDomainConfirmReachablePersistFailureDoesNotClose(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	srv.cloudflare = newFakeCF()
	boot := &fakeBootstrap{open: true}
	srv.SetBootstrapCloser(boot)

	envFile := filepath.Join(t.TempDir(), "redgres.env")
	if err := os.WriteFile(envFile, []byte("REDGRES_COOKIE_SECURE=false\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(envFile, os.O_WRONLY|os.O_TRUNC, 0)
	if err == nil {
		_ = f.Close()
		t.Skip("platform allows write on 0400 env file")
	}
	srv.cfg.EnvFile = envFile

	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, `{"zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"redgres.example.com","db":"db.example.com","rs":"rs.example.com","pgadmin":"pgadmin.example.com","redis":"redis.example.com"}}`)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/access-policy", cookie, csrf, `{"emails":["owner@example.com"]}`)); rec.Code != http.StatusOK {
		t.Fatalf("access policy = %d %s", rec.Code, rec.Body.String())
	}
	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/confirm-reachable", cookie, csrf, `{}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("confirm persist fail = %d %s", rec.Code, rec.Body.String())
	}
	if bootstrap.MarkerPresent(srv.cfg.SQLitePath) {
		t.Fatal("persist failure must not write bootstrap.closed")
	}
	if !boot.open {
		t.Fatal("persist failure must not close bootstrap")
	}
}

func TestDomainConfirmReachableProductionMissingEnvDoesNotClose(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	srv.cloudflare = newFakeCF()
	boot := &fakeBootstrap{open: true}
	srv.SetBootstrapCloser(boot)
	srv.cfg.Environment = config.EnvironmentProduction
	srv.cfg.EnvFile = filepath.Join(t.TempDir(), "missing.env")

	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, `{"zone":"example.com","origin_ip":"203.0.113.10","hostnames":{"console":"redgres.example.com","db":"db.example.com","rs":"rs.example.com","pgadmin":"pgadmin.example.com","redis":"redis.example.com"}}`)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/access-policy", cookie, csrf, `{"emails":["owner@example.com"]}`)); rec.Code != http.StatusOK {
		t.Fatalf("access policy = %d %s", rec.Code, rec.Body.String())
	}
	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/confirm-reachable", cookie, csrf, `{}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("confirm missing env = %d %s", rec.Code, rec.Body.String())
	}
	if bootstrap.MarkerPresent(srv.cfg.SQLitePath) {
		t.Fatal("missing env must not write bootstrap.closed")
	}
	if !boot.open {
		t.Fatal("missing env must not close bootstrap")
	}
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}
