package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/audit"
	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/SSujitX/redgres/internal/secrets"
)

const postgresCreatePath = "/api/v1/postgres/databases"
const postgresCreateBody = `{"database":"project_a","owner":"app_project_a"}`
const postgresRotatePath = "/api/v1/postgres/databases/project_a/credentials/rotate"

func createMemory(t *testing.T) *postgresadmin.MemoryCatalog {
	t.Helper()
	return &postgresadmin.MemoryCatalog{}
}

func createInventory(t *testing.T, cat *postgresadmin.MemoryCatalog, vaultKey string) postgresadmin.Inventory {
	t.Helper()
	return postgresadmin.NewServiceWithVaultKey(cat, postgresadmin.NewPolicy(config.Config{
		PostgresDatabase: "postgres",
		PostgresUser:     "redgres_console",
	}), vaultKey)
}

func createServer(t *testing.T, cat *postgresadmin.MemoryCatalog, vaultKey string) *Server {
	t.Helper()
	srv := testServerWithPostgres(t, createInventory(t, cat, vaultKey))
	srv.cfg.PostgresDatabase = "postgres"
	srv.cfg.PostgresUser = "redgres_console"
	srv.cfg.PostgresPublicHost = "db.example.com"
	srv.cfg.PostgresDirectPort = "5432"
	srv.cfg.PostgresPooledPort = "6432"
	return srv
}

func TestPostgresCreateRequiresSession(t *testing.T) {
	fx := loadPython49(t)
	srv := createServer(t, createMemory(t), secrets.DeriveVaultKey(fx.SessionSecret))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, postgresCreatePath, strings.NewReader(postgresCreateBody)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("401 leaked credential: %s", rec.Body.String())
	}
}

func TestPostgresCreateRequiresCSRF(t *testing.T) {
	fx := loadPython49(t)
	srv := createServer(t, createMemory(t), secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresCreatePath, cookie, "", postgresCreateBody))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeCSRFInvalid {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestPostgresCreateCapabilityIsProvision(t *testing.T) {
	if !hasCapability("postgres.provision") {
		t.Fatal("postgres.provision must be granted")
	}
	srv, _ := testServer(t, nil)
	reached := false
	handler := srv.requireCapability("postgres.export")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, postgresCreatePath, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
	if reached {
		t.Fatal("handler ran without the capability")
	}
}

func TestPostgresCreateUnknownFieldIs400(t *testing.T) {
	fx := loadPython49(t)
	cat := createMemory(t)
	srv := createServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, raw := range []string{
		`{"database":"project_a","owner":"app_project_a","role_password":"nope"}`,
		`{"database":"project_a","owner":"app_project_a","create_role":true}`,
		`{"database":"project_a","owner":"app_project_a","password":"nope"}`,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPost, postgresCreatePath, cookie, csrf, raw))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Unknown field") {
			t.Fatalf("body = %s", rec.Body.String())
		}
	}
	if cat.CreateRoleCalls != 0 {
		t.Fatal("unknown field must not DDL")
	}
	var n int
	if err := srv.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action = 'postgres.database.create'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("unknown field must not audit: %d", n)
	}
}

func TestPostgresCreateProtectedIs403NoDDL(t *testing.T) {
	fx := loadPython49(t)
	cat := createMemory(t)
	srv := createServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresCreatePath, cookie, csrf, `{"database":"postgres","owner":"app_project_a"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeProtectedResource || body.Error.Message != "This PostgreSQL name is protected" {
		t.Fatalf("error = %#v", body.Error)
	}
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseCalls != 0 {
		t.Fatal("protected must not DDL")
	}
	var n int
	if err := srv.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action = 'postgres.database.create'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("protected must not audit: %d", n)
	}
}

func TestPostgresCreateConflictDatabase(t *testing.T) {
	fx := loadPython49(t)
	cat := &postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{{Name: "project_a", Owner: "app_other", AllowConn: true}}}
	srv := createServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresCreatePath, cookie, csrf, postgresCreateBody))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeConflict || body.Error.Message != "A PostgreSQL database with this name already exists" {
		t.Fatalf("error = %#v", body.Error)
	}
	if body.Error.Fields["database"] != "exists" {
		t.Fatalf("fields = %#v", body.Error.Fields)
	}
	if cat.CreateRoleCalls != 0 {
		t.Fatal("conflict must not DDL")
	}
}

func TestPostgresCreateConflictRole(t *testing.T) {
	fx := loadPython49(t)
	cat := &postgresadmin.MemoryCatalog{ExistingRoles: []string{"app_project_a"}}
	srv := createServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresCreatePath, cookie, csrf, postgresCreateBody))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeConflict || body.Error.Message != "A PostgreSQL role with this name already exists" {
		t.Fatalf("error = %#v", body.Error)
	}
	if body.Error.Fields["owner"] != "exists" {
		t.Fatalf("fields = %#v", body.Error.Fields)
	}
}

func TestPostgresCreateMissingVaultKeyIs503NoDDL(t *testing.T) {
	cat := createMemory(t)
	srv := createServer(t, cat, "")
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresCreatePath, cookie, csrf, postgresCreateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PostgreSQL is unavailable") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if cat.CreateRoleCalls != 0 {
		t.Fatal("missing vault key must not DDL")
	}
}

func TestPostgresCreateVaultInsertFailureCompensates(t *testing.T) {
	fx := loadPython49(t)
	canary := "postgresql://canary-token:secret@10.0.0.1/db"
	cat := &postgresadmin.MemoryCatalog{InsertCredentialErr: errors.New(canary)}
	srv := createServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresCreatePath, cookie, csrf, postgresCreateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "canary-token") || strings.Contains(rec.Body.String(), fx.ASCII.Token) || strings.Contains(rec.Body.String(), `"credential"`) {
		t.Fatalf("leaked canary: %s", rec.Body.String())
	}
	if cat.DropDatabaseCalls != 1 || cat.DropRoleCalls != 1 {
		t.Fatalf("compensate drops db=%d role=%d", cat.DropDatabaseCalls, cat.DropRoleCalls)
	}
}

func TestPostgresCreateRoleThenDatabaseFailDropsRoleOnly(t *testing.T) {
	fx := loadPython49(t)
	cat := &postgresadmin.MemoryCatalog{CreateDatabaseErr: postgresadmin.ErrUnavailable}
	srv := createServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresCreatePath, cookie, csrf, postgresCreateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if cat.DropRoleCalls != 1 || cat.DropDatabaseCalls != 0 {
		t.Fatalf("drops role=%d db=%d", cat.DropRoleCalls, cat.DropDatabaseCalls)
	}
}

func TestPostgresCreate201NoStoreOneTimeFalse(t *testing.T) {
	fx := loadPython49(t)
	cat := createMemory(t)
	srv := createServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresCreatePath, cookie, csrf, postgresCreateBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store, max-age=0" {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("pragma = %q", rec.Header().Get("Pragma"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	resource, _ := body["resource"].(map[string]any)
	if resource["type"] != "postgres_database" || resource["name"] != "project_a" {
		t.Fatalf("resource = %#v", body["resource"])
	}
	cred, _ := body["credential"].(map[string]any)
	if cred["username"] != "app_project_a" {
		t.Fatalf("username = %#v", cred["username"])
	}
	password, _ := cred["password"].(string)
	if len(password) != 32 {
		t.Fatalf("password length = %d", len(password))
	}
	if cred["one_time"] != false {
		t.Fatalf("one_time = %#v", cred["one_time"])
	}
	urls, _ := cred["urls"].(map[string]any)
	direct, _ := urls["direct"].(string)
	if !strings.HasPrefix(direct, "postgresql://app_project_a:") || !strings.HasSuffix(direct, "@db.example.com:5432/project_a?sslmode=require") {
		t.Fatalf("direct = %#v", urls["direct"])
	}
	pooled, _ := urls["pooled"].(string)
	if !strings.HasPrefix(pooled, "postgresql://app_project_a:") || !strings.HasSuffix(pooled, "@db.example.com:6432/project_a?sslmode=require") {
		t.Fatalf("pooled = %#v", urls["pooled"])
	}
	id, _ := body["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'postgres.database.create' AND outcome = 'success' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "project_a" || meta["owner"] != "app_project_a" {
		t.Fatalf("metadata = %s", metadata)
	}
	if len(meta) != 2 {
		t.Fatalf("metadata keys = %#v", meta)
	}
	if strings.Contains(metadata, password) || strings.Contains(metadata, "postgresql://") || strings.Contains(metadata, cat.Ciphertexts["app_project_a"]) {
		t.Fatalf("audit leaked secret: %s", metadata)
	}
	list := httptest.NewRecorder()
	h.ServeHTTP(list, authed(http.MethodGet, postgresCreatePath, cookie, csrf, ""))
	if list.Code != http.StatusOK {
		t.Fatalf("GET list status = %d %s", list.Code, list.Body.String())
	}
}

func TestPostgresCreateAuditFailClosed(t *testing.T) {
	fx := loadPython49(t)
	cat := createMemory(t)
	srv := createServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	dead, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dead-audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dead.Close() })
	_ = dead.Close()
	srv.audit = audit.Store{DB: dead}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresCreatePath, cookie, csrf, postgresCreateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("audit failure returned credential: %s", rec.Body.String())
	}
	if cat.InsertCalls != 1 {
		t.Fatalf("cluster+vault must remain: inserts=%d drops db=%d role=%d", cat.InsertCalls, cat.DropDatabaseCalls, cat.DropRoleCalls)
	}
	if cat.DropDatabaseCalls != 0 || cat.DropRoleCalls != 0 {
		t.Fatal("audit-fail must not compensate")
	}
}

func TestPostgresRotatePathIsRegistered(t *testing.T) {
	fx := loadPython49(t)
	cat := &postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "project_a", Owner: "app_project_a", AllowConn: true, OwnerCanLogin: true},
	}}
	srv := createServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(method, postgresRotatePath, cookie, csrf, postgresRotateBody))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d %s", method, rec.Code, rec.Body.String())
		}
	}
	post := httptest.NewRecorder()
	h.ServeHTTP(post, authed(http.MethodPost, postgresRotatePath, cookie, csrf, postgresRotateBody))
	if post.Code != http.StatusOK {
		t.Fatalf("POST rotate status = %d %s", post.Code, post.Body.String())
	}
	item := httptest.NewRecorder()
	h.ServeHTTP(item, authed(http.MethodPost, "/api/v1/postgres/databases/project_a", cookie, csrf, postgresCreateBody))
	if item.Code != http.StatusMethodNotAllowed {
		t.Fatalf("item POST status = %d %s", item.Code, item.Body.String())
	}
}

func TestPostgresCreateNilAdapterAuditsFailure(t *testing.T) {
	srv, _ := testServer(t, nil)
	srv.cfg.PostgresDatabase = "postgres"
	srv.cfg.PostgresUser = "redgres_console"
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresCreatePath, cookie, csrf, postgresCreateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) {
		t.Fatalf("503 leaked credential: %s", rec.Body.String())
	}
	var outcome string
	if err := srv.db.QueryRow(`SELECT outcome FROM audit_events WHERE action = 'postgres.database.create' ORDER BY id DESC LIMIT 1`).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome != "failure" {
		t.Fatalf("outcome = %s", outcome)
	}
}
