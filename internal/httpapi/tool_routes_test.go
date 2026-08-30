package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/toolgate"
)

func TestToolLaunchRequiresConfiguredURL(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	handler := srv.Handler()
	cookie, csrf := login(t, handler)
	rec := serve(handler, authed(http.MethodPost, "/api/v1/tools/pgadmin/launch", cookie, csrf, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unconfigured launch = %d %s", rec.Code, rec.Body.String())
	}
}

func TestToolLaunchReturnsURLWithoutLoggingTicket(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	srv.cfg.PgAdminURL = "https://pgadmin.example.com"
	handler := srv.Handler()
	cookie, csrf := login(t, handler)
	rec := serve(handler, authed(http.MethodPost, "/api/v1/tools/pgadmin/launch", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("launch = %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	launch, _ := body["launch_url"].(string)
	if !strings.HasPrefix(launch, "https://pgadmin.example.com"+toolgate.LaunchPath+"?ticket=") {
		t.Fatalf("launch_url = %q", launch)
	}
	ticket := strings.TrimPrefix(launch, "https://pgadmin.example.com"+toolgate.LaunchPath+"?ticket=")
	if len(ticket) < 32 {
		t.Fatalf("ticket too short")
	}
	if rec.Header().Get("Cache-Control") == "" || !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'tools.launch' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata, ticket) {
		t.Fatalf("audit leaked ticket: %s", metadata)
	}
}

func TestToolLaunchUnknownTool(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	handler := srv.Handler()
	cookie, csrf := login(t, handler)
	rec := serve(handler, authed(http.MethodPost, "/api/v1/tools/other/launch", cookie, csrf, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown tool = %d %s", rec.Code, rec.Body.String())
	}
}

func TestToolLaunchUnauthorized(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := serve(srv.Handler(), authed(http.MethodPost, "/api/v1/tools/pgadmin/launch", "", "", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth launch = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRedisInsightLaunch(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	srv.cfg.RedisInsightURL = "https://redis.example.com"
	handler := srv.Handler()
	cookie, csrf := login(t, handler)
	rec := serve(handler, authed(http.MethodPost, "/api/v1/tools/redisinsight/launch", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("launch = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "https://redis.example.com"+toolgate.LaunchPath+"?ticket=") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPgAdminReveal(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	passPath := filepath.Join(t.TempDir(), "pgadmin.pass")
	if err := os.WriteFile(passPath, []byte("pgadmin-canary-password-32chars!!\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv.cfg.PgAdminEmail = "admin@redgres.com"
	srv.cfg.PgAdminPasswordFile = passPath
	handler := srv.Handler()
	cookie, csrf := login(t, handler)
	rec := serve(handler, authed(http.MethodPost, "/api/v1/tools/pgadmin/credentials/reveal", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("reveal = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "pgadmin-canary-password-32chars!!") {
		t.Fatalf("body missing password")
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'tools.pgadmin.reveal' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata, "pgadmin-canary-password-32chars!!") {
		t.Fatalf("audit leaked password: %s", metadata)
	}
}

func TestPgAdminRevealRejectsOversizedFile(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	passPath := filepath.Join(t.TempDir(), "pgadmin.pass")
	if err := os.WriteFile(passPath, []byte(strings.Repeat("x", maxPgAdminPasswordBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	srv.cfg.PgAdminEmail = "admin@redgres.com"
	srv.cfg.PgAdminPasswordFile = passPath
	handler := srv.Handler()
	cookie, csrf := login(t, handler)
	rec := serve(handler, authed(http.MethodPost, "/api/v1/tools/pgadmin/credentials/reveal", cookie, csrf, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("oversized reveal = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), passPath) {
		t.Fatalf("reveal echoed path: %s", rec.Body.String())
	}
}

func TestPgAdminRevealRejectsDirectory(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	dir := t.TempDir()
	srv.cfg.PgAdminEmail = "admin@redgres.com"
	srv.cfg.PgAdminPasswordFile = dir
	handler := srv.Handler()
	cookie, csrf := login(t, handler)
	rec := serve(handler, authed(http.MethodPost, "/api/v1/tools/pgadmin/credentials/reveal", cookie, csrf, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("directory reveal = %d %s", rec.Code, rec.Body.String())
	}
}
