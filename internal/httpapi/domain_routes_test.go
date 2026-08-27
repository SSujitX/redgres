package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/cloudflare"
)

type fakeCF struct {
	zone              cloudflare.Zone
	tunnels           map[string]cloudflare.Tunnel
	records           map[string]cloudflare.Record
	apps              map[string]cloudflare.AccessApp
	deletedT          []string
	deletedR          []string
	deletedA          []string
	verifyErr         error
	lastIngressHost   string
	lastIngressOrigin string
}

func newFakeCF() *fakeCF {
	return &fakeCF{
		zone:    cloudflare.Zone{ID: "zone-1", Name: "example.com", AccountID: "acct-1"},
		tunnels: map[string]cloudflare.Tunnel{},
		records: map[string]cloudflare.Record{},
		apps:    map[string]cloudflare.AccessApp{},
	}
}

func (f *fakeCF) DiscoverZone(_ context.Context, name string) (cloudflare.Zone, error) {
	if name != "example.com" {
		return cloudflare.Zone{}, cloudflare.ErrNotFound
	}
	return f.zone, nil
}

func (f *fakeCF) CreateTunnel(_ context.Context, accountID, name string) (cloudflare.Tunnel, error) {
	t := cloudflare.Tunnel{ID: "tun-1", Name: name, Token: "fake-tunnel-token"}
	f.tunnels[t.ID] = t
	return t, nil
}

func (f *fakeCF) ConfigureIngress(_ context.Context, accountID, tunnelID, hostname, originHTTP string) error {
	if _, ok := f.tunnels[tunnelID]; !ok {
		return cloudflare.ErrNotFound
	}
	f.lastIngressHost = hostname
	f.lastIngressOrigin = originHTTP
	return nil
}

func (f *fakeCF) CreateRecord(_ context.Context, zoneID, name, content string, proxied bool) (cloudflare.Record, error) {
	r := cloudflare.Record{ID: "rec-1", Name: name}
	f.records[r.ID] = r
	return r, nil
}

func (f *fakeCF) CreateAccessApp(_ context.Context, accountID, domain string) (cloudflare.AccessApp, error) {
	a := cloudflare.AccessApp{ID: "app-1", Domain: domain}
	f.apps[a.ID] = a
	return a, nil
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

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, `{"zone":"example.com","hostname":"console.example.com"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"bootstrap_still_open":true`) {
		t.Fatalf("expected bootstrap_still_open=true: %s", rec.Body.String())
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
	if fake.lastIngressHost != "console.example.com" || fake.lastIngressOrigin != "http://127.0.0.1:8790" {
		t.Fatalf("ingress host=%q origin=%q", fake.lastIngressHost, fake.lastIngressOrigin)
	}

	rec = serve(handler, authed(http.MethodGet, "/api/v1/domain", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("domain get = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"configured":true`) {
		t.Fatalf("expected configured=true: %s", rec.Body.String())
	}
}

func TestDomainDisconnectDeletesOnlyCreated(t *testing.T) {
	srv, handler, cookie, csrf, tokenPath := newDomainServer(t)
	if err := writeTokenFile(tokenPath, "test-token"); err != nil {
		t.Fatal(err)
	}
	fake := newFakeCF()
	srv.cloudflare = fake

	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, `{"zone":"example.com","hostname":"console.example.com"}`)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}

	rec := serve(handler, authed(http.MethodDelete, "/api/v1/domain", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("disconnect = %d %s", rec.Code, rec.Body.String())
	}
	if len(fake.deletedA) != 1 || fake.deletedA[0] != "app-1" {
		t.Fatalf("deleted apps = %v", fake.deletedA)
	}
	if len(fake.deletedR) != 1 || fake.deletedR[0] != "rec-1" {
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

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, `{"zone":"example.com","hostname":"console.example.com"}`))
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

	body := `{"zone":"example.com","hostname":"console.example.com"}`
	if rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, body)); rec.Code != http.StatusOK {
		t.Fatalf("first apply = %d %s", rec.Code, rec.Body.String())
	}
	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("reapply = %d %s", rec.Code, rec.Body.String())
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

	rec := serve(handler, authed(http.MethodPost, "/api/v1/domain/apply", cookie, csrf, `{"zone":"example.com","hostname":"console.example.com"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if len(fake.deletedA) != 1 || fake.deletedA[0] != "app-1" {
		t.Fatalf("compensation did not delete access app: %v", fake.deletedA)
	}
	if len(fake.deletedR) != 1 || fake.deletedR[0] != "rec-1" {
		t.Fatalf("compensation did not delete record: %v", fake.deletedR)
	}
	if len(fake.deletedT) != 1 || fake.deletedT[0] != "tun-1" {
		t.Fatalf("compensation did not delete tunnel: %v", fake.deletedT)
	}
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}
