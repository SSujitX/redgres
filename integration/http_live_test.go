package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/internal/httpapi"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/SSujitX/redgres/internal/redisadmin"
	"github.com/SSujitX/redgres/migrations"
	"github.com/redis/go-redis/v9"
)

const (
	liveOwnerPassword = "owner-secret-15"
	httpLiveDB        = "http_live"
	httpLiveOwner     = "app_http_live"
	httpLiveRedisUser = "live_http_ro"
)

func liveLogin(t *testing.T, h http.Handler) (cookie, csrf string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"username":"admin","password":"`+liveOwnerPassword+`"}`))
	req.Header.Set("Origin", "http://127.0.0.1:8790")
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	csrf, _ = body["csrf_token"].(string)
	for _, c := range rec.Result().Cookies() {
		if c.Name == "redgres_session" {
			cookie = c.Value
			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
				t.Fatalf("session cookie flags = %+v", c)
			}
		}
	}
	if cookie == "" || csrf == "" {
		t.Fatal("missing session cookie or csrf")
	}
	return cookie, csrf
}

func liveAuthed(t *testing.T, h http.Handler, method, path, cookie, csrf, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "redgres_session", Value: cookie})
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	req.Header.Set("Origin", "http://127.0.0.1:8790")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode %d %s: %v", rec.Code, rec.Body.String(), err)
	}
}

func redisAddr(t *testing.T) string {
	t.Helper()
	urlFile, ok := liveRedisEnv(t)
	if !ok {
		t.Skip(skipLiveEnv)
	}
	raw, err := os.ReadFile(urlFile)
	if err != nil {
		t.Fatal(err)
	}
	opts, err := redis.ParseURL(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return opts.Addr
}

func statusStates(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body struct {
		Components []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"components"`
	}
	decodeBody(t, rec, &body)
	out := make(map[string]string, len(body.Components))
	for _, c := range body.Components {
		out[c.ID] = c.State
	}
	return out
}

// TestLiveHTTPOverRealServices drives the HTTP API of an in-process Redgres
// server wired to real PostgreSQL (with provisioned vault), real Redis, and a
// real SQLite control-plane database: login flags, CSRF, status, PostgreSQL
// create/list/details/connection/reveal/rotate, rows-delete/truncate with
// AUTH-006 owner-password reauth, the DROP backup-catalog 503 gate, the Redis
// ACL user lifecycle, and audit visibility.
func TestLiveHTTPOverRealServices(t *testing.T) {
	clearInheritedRedgresEnv(t)
	pgHost, pgPort, pgPassFile, pgOK := livePostgresEnv(t)
	redisURLFile, redisOK := liveRedisEnv(t)
	if !pgOK || !redisOK {
		t.Skip(skipLiveEnv)
	}
	provisionVault(t, pgHost, pgPort, pgPassFile)
	seedClean(t, pgHost, pgPort, pgPassFile)

	secretFile := filepath.Join(t.TempDir(), "vault-secret")
	if err := os.WriteFile(secretFile, []byte(testVaultSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "live-http.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(db, migrations.FS); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		Environment:              config.EnvironmentDevelopment,
		Address:                  "127.0.0.1:8790",
		BaseURL:                  "http://127.0.0.1:8790",
		SessionTTL:               12 * time.Hour,
		AbsoluteSessionTTL:       24 * time.Hour,
		PostgresHost:             pgHost,
		PostgresPort:             pgPort,
		PostgresDatabase:         "postgres",
		PostgresUser:             "postgres",
		PostgresPasswordFile:     pgPassFile,
		PostgresSSLMode:          "prefer",
		PostgresExpectedMajor:    livePostgresExpectedMajor(t),
		PostgresPublicHost:       "db.example.com",
		PostgresDirectPort:       "5432",
		PostgresPooledPort:       "6432",
		LegacyVaultSecretFile:    secretFile,
		RedisAdminURLFile:        redisURLFile,
		RedisExpectedSeries:      liveRedisExpectedSeries(t),
		FeaturePostgresRowDelete: true,
		FeaturePostgresTruncate:  true,
		FeaturePostgresDrop:      true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pgSvc, pgCloser, err := postgresadmin.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("postgresadmin.Open: %v", err)
	}
	t.Cleanup(pgCloser)
	rdSvc, rdCloser, err := redisadmin.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("redisadmin.Open: %v", err)
	}
	if rdCloser != nil {
		t.Cleanup(rdCloser)
	}

	srv := httpapi.New(cfg, db, fstest.MapFS{}, nil, pgSvc, rdSvc)
	if _, err := auth.CreateOrReplaceOwner(db, "admin", liveOwnerPassword, false); err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	cookie, csrf := liveLogin(t, h)

	// --- PLAT-001: live component status ---
	states := statusStates(t, liveAuthed(t, h, http.MethodGet, "/api/v1/status", cookie, csrf, ""))
	if states["postgres_direct"] != "ok" || states["redis"] != "ok" || states["redgres_state"] != "ok" {
		t.Fatalf("status states = %v", states)
	}

	// --- AUTH-004: mutation without CSRF is rejected ---
	noCSRF := liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases", cookie, "",
		`{"database":"should_not_exist","owner":"app_x"}`)
	if noCSRF.Code != http.StatusForbidden {
		t.Fatalf("no-CSRF create status = %d", noCSRF.Code)
	}

	// --- PG-003 over HTTP: create returns the credential with no-store ---
	rec := liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases", cookie, csrf,
		`{"database":"`+httpLiveDB+`","owner":"`+httpLiveOwner+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("create Cache-Control = %q", cc)
	}
	var created struct {
		Resource struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"resource"`
		Credential struct {
			Username string `json:"username"`
			Password string `json:"password"`
			OneTime  bool   `json:"one_time"`
			URLs     *struct {
				Direct string `json:"direct"`
				Pooled string `json:"pooled"`
			} `json:"urls"`
		} `json:"credential"`
	}
	decodeBody(t, rec, &created)
	if created.Resource.Name != httpLiveDB || created.Credential.Username != httpLiveOwner || created.Credential.Password == "" {
		t.Fatalf("created = %+v", created)
	}
	if created.Credential.OneTime {
		t.Fatal("PG create credential must not be one_time")
	}
	if created.Credential.URLs == nil || !strings.Contains(created.Credential.URLs.Direct, "db.example.com") || !strings.Contains(created.Credential.URLs.Pooled, "db.example.com") {
		t.Fatalf("urls = %+v", created.Credential.URLs)
	}

	// --- PG-001 list + PG-002 details ---
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/postgres/databases", cookie, csrf, "")
	var listed struct {
		Databases []struct {
			Name string `json:"name"`
		} `json:"databases"`
	}
	decodeBody(t, rec, &listed)
	found := false
	for _, item := range listed.Databases {
		if item.Name == httpLiveDB {
			found = true
		}
	}
	if !found {
		t.Fatalf("list missing %s: %+v", httpLiveDB, listed.Databases)
	}
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/postgres/databases/"+httpLiveDB, cookie, csrf, "")
	var det struct {
		Database struct {
			Owner string `json:"owner"`
		} `json:"database"`
	}
	decodeBody(t, rec, &det)
	if det.Database.Owner != httpLiveOwner {
		t.Fatalf("details owner = %q", det.Database.Owner)
	}

	// --- PG-004 masked connection + PG-005 reveal (round-trip decrypt) ---
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/postgres/databases/"+httpLiveDB+"/connection", cookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("connection status %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), created.Credential.Password) || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatal("masked connection leaked the password")
	}
	if !strings.Contains(rec.Body.String(), "masked_direct_url") {
		t.Fatalf("masked connection missing URL: %s", rec.Body.String())
	}
	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases/"+httpLiveDB+"/connection/reveal", cookie, csrf, "")
	var revealed struct {
		Credential struct {
			Password string `json:"password"`
		} `json:"credential"`
	}
	decodeBody(t, rec, &revealed)
	if revealed.Credential.Password != created.Credential.Password {
		t.Fatalf("reveal password mismatch (Fernet round-trip failed)")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("reveal Cache-Control = %q", cc)
	}

	// --- PG-006 rotate over HTTP ---
	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases/"+httpLiveDB+"/credentials/rotate", cookie, csrf,
		`{"confirmation":"`+httpLiveDB+`"}`)
	var rotated struct {
		Credential struct {
			Password string `json:"password"`
		} `json:"credential"`
	}
	decodeBody(t, rec, &rotated)
	if rotated.Credential.Password == "" || rotated.Credential.Password == created.Credential.Password {
		t.Fatal("rotate did not issue a new password")
	}

	// --- seed a table for destructive paths ---
	appConn := livePGConn(t, pgHost, pgPort, httpLiveDB, httpLiveOwner, rotated.Credential.Password)
	if _, err := appConn.Exec(context.Background(), "CREATE TABLE public.items (id integer PRIMARY KEY, name text NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := appConn.Exec(context.Background(), "INSERT INTO public.items (id, name) VALUES (1,'a'),(2,'b')"); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	// --- PG-008 row delete with AUTH-006 reauth ---
	rowsPath := "/api/v1/postgres/databases/" + httpLiveDB + "/tables/public/items/rows"
	bad := liveAuthed(t, h, http.MethodDelete, rowsPath, cookie, csrf,
		`{"table_confirmation":"items","owner_password":"wrong-password","primary_key_values":[1]}`)
	if bad.Code != http.StatusForbidden {
		t.Fatalf("row delete wrong reauth status = %d: %s", bad.Code, bad.Body.String())
	}
	var badErr struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeBody(t, bad, &badErr)
	if badErr.Error.Code != "reauth_required" {
		t.Fatalf("row delete wrong reauth code = %q", badErr.Error.Code)
	}
	ok := liveAuthed(t, h, http.MethodDelete, rowsPath, cookie, csrf,
		`{"table_confirmation":"items","owner_password":"`+liveOwnerPassword+`","primary_key_values":[1]}`)
	if ok.Code != http.StatusOK {
		t.Fatalf("row delete status %d: %s", ok.Code, ok.Body.String())
	}
	var deletedBody struct {
		Deleted int64 `json:"deleted"`
	}
	decodeBody(t, ok, &deletedBody)
	if deletedBody.Deleted != 1 {
		t.Fatalf("row delete count = %d", deletedBody.Deleted)
	}

	// --- PG-009 truncate with AUTH-006 reauth ---
	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases/"+httpLiveDB+"/truncate", cookie, csrf,
		`{"database_confirmation":"`+httpLiveDB+`","owner_password":"`+liveOwnerPassword+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("truncate status %d: %s", rec.Code, rec.Body.String())
	}

	// --- PG-011 DROP backup-gate 503 with a real PG target ---
	rec = liveAuthed(t, h, http.MethodDelete, "/api/v1/postgres/databases/"+httpLiveDB, cookie, csrf,
		`{"database_confirmation":"`+httpLiveDB+`","owner_password":"`+liveOwnerPassword+`"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("drop without catalog status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Backup catalog is not configured") {
		t.Fatalf("drop 503 message = %s", rec.Body.String())
	}

	// --- PG-012 security overview over HTTP ---
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/postgres/security", cookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("security status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), httpLiveDB) {
		t.Fatalf("security overview missing %s", httpLiveDB)
	}

	// --- REDIS lifecycle over HTTP ---
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/redis/users", cookie, csrf, "")
	var usersBody struct {
		State string `json:"state"`
	}
	decodeBody(t, rec, &usersBody)
	if usersBody.State != "ok" {
		t.Fatalf("redis users state = %q", usersBody.State)
	}
	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/redis/users", cookie, csrf,
		`{"username":"`+httpLiveRedisUser+`","key_pattern":"live:http","preset":"read-only"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("redis create status %d: %s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("redis create Cache-Control = %q", cc)
	}
	var redisCreated struct {
		Credential struct {
			Username string `json:"username"`
			Password string `json:"password"`
			OneTime  bool   `json:"one_time"`
		} `json:"credential"`
	}
	decodeBody(t, rec, &redisCreated)
	if redisCreated.Credential.Username != httpLiveRedisUser || redisCreated.Credential.Password == "" || !redisCreated.Credential.OneTime {
		t.Fatalf("redis created = %+v", redisCreated.Credential)
	}
	// REDIS-002 inspect fidelity over HTTP (real ACL parse).
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/redis/users/"+httpLiveRedisUser, cookie, csrf, "")
	var redisUser struct {
		User struct {
			Preset       string `json:"preset"`
			RuleFidelity string `json:"rule_fidelity"`
			Enabled      bool   `json:"enabled"`
		} `json:"user"`
	}
	decodeBody(t, rec, &redisUser)
	if redisUser.User.Preset != "read-only" || redisUser.User.RuleFidelity != "exact" || !redisUser.User.Enabled {
		t.Fatalf("redis inspect = %+v", redisUser.User)
	}
	// PATCH preserves the password (verify by connecting with the old one).
	rec = liveAuthed(t, h, http.MethodPatch, "/api/v1/redis/users/"+httpLiveRedisUser, cookie, csrf,
		`{"key_pattern":"live:new","preset":"cache-read-write"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("redis patch status %d: %s", rec.Code, rec.Body.String())
	}
	var redisPatched struct {
		User struct {
			KeyPattern string `json:"key_pattern"`
			Preset     string `json:"preset"`
		} `json:"user"`
	}
	decodeBody(t, rec, &redisPatched)
	if redisPatched.User.KeyPattern != "live:new:*" || redisPatched.User.Preset != "cache-read-write" {
		t.Fatalf("redis patched = %+v", redisPatched.User)
	}
	redisClient := liveRedisClient(t, redisAddr(t), httpLiveRedisUser, redisCreated.Credential.Password)
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pingCancel()
	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		t.Fatalf("redis old password rejected after PATCH: %v", err)
	}
	// rotate
	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/redis/users/"+httpLiveRedisUser+"/credentials/rotate", cookie, csrf, "")
	var redisRotated struct {
		Credential struct {
			Password string `json:"password"`
		} `json:"credential"`
	}
	decodeBody(t, rec, &redisRotated)
	if redisRotated.Credential.Password == "" || redisRotated.Credential.Password == redisCreated.Credential.Password {
		t.Fatal("redis rotate did not issue a new password")
	}
	// enable/disable
	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/redis/users/"+httpLiveRedisUser+"/disable", cookie, csrf, "")
	var redisDisabled struct {
		User struct {
			Enabled bool `json:"enabled"`
		} `json:"user"`
	}
	decodeBody(t, rec, &redisDisabled)
	if redisDisabled.User.Enabled {
		t.Fatal("disable reported enabled")
	}
	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/redis/users/"+httpLiveRedisUser+"/enable", cookie, csrf, "")
	var redisEnabled struct {
		User struct {
			Enabled bool `json:"enabled"`
		} `json:"user"`
	}
	decodeBody(t, rec, &redisEnabled)
	if !redisEnabled.User.Enabled {
		t.Fatal("enable reported disabled")
	}
	// delete with AUTH-006 reauth
	rec = liveAuthed(t, h, http.MethodDelete, "/api/v1/redis/users/"+httpLiveRedisUser, cookie, csrf,
		`{"username_confirmation":"`+httpLiveRedisUser+`","owner_password":"`+liveOwnerPassword+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("redis delete status %d: %s", rec.Code, rec.Body.String())
	}

	// --- PLAT-002/003: audit shows success and failure events ---
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/audit?limit=100", cookie, csrf, "")
	var audit struct {
		Events []struct {
			Action  string `json:"action"`
			Target  string `json:"target"`
			Outcome string `json:"outcome"`
		} `json:"events"`
	}
	decodeBody(t, rec, &audit)
	seen := map[string]bool{}
	for _, ev := range audit.Events {
		seen[ev.Action+"/"+ev.Outcome] = true
	}
	for _, want := range []string{"postgres.database.create/success", "redis.user.create/success", "postgres.rows.delete/failure", "redis.user.delete/success"} {
		if !seen[want] {
			t.Fatalf("audit missing %s (have %d events)", want, len(audit.Events))
		}
	}
}
