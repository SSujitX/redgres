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
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/SSujitX/redgres/internal/secrets"
)

const postgresRotateBody = `{"confirmation":"project_a"}`
const rotateCanaryPassword = "canary-rotate-password-secret"

func rotateRow() postgresadmin.CatalogRow {
	return postgresadmin.CatalogRow{Name: "project_a", Owner: "app_project_a", AllowConn: true, OwnerCanLogin: true}
}

func rotateMemory(t *testing.T) *postgresadmin.MemoryCatalog {
	t.Helper()
	return &postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{rotateRow()}}
}

func rotateServer(t *testing.T, cat *postgresadmin.MemoryCatalog, vaultKey string) *Server {
	t.Helper()
	return createServer(t, cat, vaultKey)
}

func TestPostgresRotateRequiresSession(t *testing.T) {
	fx := loadPython49(t)
	srv := rotateServer(t, rotateMemory(t), secrets.DeriveVaultKey(fx.SessionSecret))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, postgresRotatePath, strings.NewReader(postgresRotateBody)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("401 leaked credential: %s", rec.Body.String())
	}
}

func TestPostgresRotateRequiresCSRF(t *testing.T) {
	fx := loadPython49(t)
	srv := rotateServer(t, rotateMemory(t), secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRotatePath, cookie, "", postgresRotateBody))
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

func TestPostgresRotateCapabilityIsCredentialsNotProvision(t *testing.T) {
	if !hasCapability("postgres.credentials") {
		t.Fatal("postgres.credentials must be granted")
	}
	if !hasCapability("postgres.provision") {
		t.Fatal("provision is also granted; rotate must still gate on credentials")
	}
	srv, _ := testServer(t, nil)
	reached := false
	handler := srv.requireCapability("postgres.export")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, postgresRotatePath, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
	if reached {
		t.Fatal("handler ran without the capability")
	}
}

func TestPostgresRotateSuccessEnvelope(t *testing.T) {
	fx := loadPython49(t)
	cat := rotateMemory(t)
	srv := rotateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRotatePath, cookie, csrf, postgresRotateBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store, max-age=0" {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("pragma = %q", rec.Header().Get("Pragma"))
	}
	if !strings.Contains(rec.Body.String(), `"one_time":false`) {
		t.Fatalf("one_time must be JSON false: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"warning"`) {
		t.Fatal("must not copy sibling warning field")
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	resource, _ := body["resource"].(map[string]any)
	if resource["type"] != "postgres_database" || resource["name"] != "project_a" {
		t.Fatalf("resource = %#v", resource)
	}
	cred, _ := body["credential"].(map[string]any)
	password, _ := cred["password"].(string)
	if cred["username"] != "app_project_a" || password == "" || cred["one_time"] != false {
		t.Fatalf("credential = %#v", cred)
	}
	urls, _ := cred["urls"].(map[string]any)
	if urls["direct"] != "postgresql://app_project_a:"+password+"@db.example.com:5432/project_a?sslmode=require" {
		t.Fatalf("direct = %#v", urls["direct"])
	}
	if urls["pooled"] != "postgresql://app_project_a:"+password+"@db.example.com:6432/project_a?sslmode=require" {
		t.Fatalf("pooled = %#v", urls["pooled"])
	}
	id, _ := body["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'postgres.credential.rotate' AND outcome = 'success' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
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
	if strings.Contains(metadata, password) || strings.Contains(metadata, "postgresql://") {
		t.Fatalf("audit leaked secret: %s", metadata)
	}
}

func TestPostgresRotateUnknownFieldIs400NoAudit(t *testing.T) {
	fx := loadPython49(t)
	cat := rotateMemory(t)
	srv := rotateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	raw := `{"confirmation":"project_a","password":"` + rotateCanaryPassword + `"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRotatePath, cookie, csrf, raw))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Unknown field") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), rotateCanaryPassword) {
		t.Fatalf("error JSON leaked canary: %s", rec.Body.String())
	}
	if cat.AlterRolePasswordCalls != 0 {
		t.Fatal("unknown field must not ALTER")
	}
	var n int
	if err := srv.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action = 'postgres.credential.rotate'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("audit count = %d", n)
	}
}

func TestPostgresRotateConfirmationMismatchIs400NoAudit(t *testing.T) {
	fx := loadPython49(t)
	cat := rotateMemory(t)
	srv := rotateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRotatePath, cookie, csrf, `{"confirmation":"other"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeValidationError || body.Error.Message != postgresRotateConfirmMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if body.Error.Fields["confirmation"] != "invalid" {
		t.Fatalf("fields = %#v", body.Error.Fields)
	}
	if cat.AlterRolePasswordCalls != 0 {
		t.Fatal("mismatch must not ALTER")
	}
	var n int
	if err := srv.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action = 'postgres.credential.rotate'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("audit count = %d", n)
	}
}

func TestPostgresRotateProtectedAndMissing(t *testing.T) {
	fx := loadPython49(t)
	key := secrets.DeriveVaultKey(fx.SessionSecret)
	srv := rotateServer(t, &postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "postgres", Owner: "app_project_a", AllowConn: true, OwnerCanLogin: true},
		{Name: "project_a", Owner: "postgres", AllowConn: true, OwnerCanLogin: true},
	}}, key)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)

	notFound := httptest.NewRecorder()
	h.ServeHTTP(notFound, authed(http.MethodPost, "/api/v1/postgres/databases/postgres/credentials/rotate", cookie, csrf, `{"confirmation":"postgres"}`))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("protected db status = %d %s", notFound.Code, notFound.Body.String())
	}
	var nf errorBody
	if err := json.Unmarshal(notFound.Body.Bytes(), &nf); err != nil {
		t.Fatal(err)
	}
	if nf.Error.Code != CodeNotFound {
		t.Fatalf("code = %q", nf.Error.Code)
	}

	forbidden := httptest.NewRecorder()
	h.ServeHTTP(forbidden, authed(http.MethodPost, postgresRotatePath, cookie, csrf, postgresRotateBody))
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("owner-denied status = %d %s", forbidden.Code, forbidden.Body.String())
	}
	var fb errorBody
	if err := json.Unmarshal(forbidden.Body.Bytes(), &fb); err != nil {
		t.Fatal(err)
	}
	if fb.Error.Code != CodeProtectedResource || fb.Error.Message != "This PostgreSQL name is protected" {
		t.Fatalf("error = %#v", fb.Error)
	}

	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, authed(http.MethodPost, "/api/v1/postgres/databases/missing_db/credentials/rotate", cookie, csrf, `{"confirmation":"missing_db"}`))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d %s", missing.Code, missing.Body.String())
	}
}

func TestPostgresRotateAuditMetadataOnly(t *testing.T) {
	fx := loadPython49(t)
	cat := rotateMemory(t)
	srv := rotateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRotatePath, cookie, csrf, postgresRotateBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	password, _ := body["credential"].(map[string]any)["password"].(string)
	var action, outcome, metadata string
	if err := srv.db.QueryRow(`SELECT action, outcome, metadata FROM audit_events ORDER BY id DESC LIMIT 1`).Scan(&action, &outcome, &metadata); err != nil {
		t.Fatal(err)
	}
	if action != "postgres.credential.rotate" || outcome != "success" {
		t.Fatalf("audit = %s %s", action, outcome)
	}
	if strings.Contains(metadata, password) || strings.Contains(metadata, rotateCanaryPassword) {
		t.Fatalf("audit leaked password: %s", metadata)
	}
}

func TestPostgresRotateVaultFailAfterAlterIs503NoCredential(t *testing.T) {
	fx := loadPython49(t)
	canary := "postgresql://canary-rotate-secret@10.0.0.1/db"
	cat := &postgresadmin.MemoryCatalog{
		Rows:                []postgresadmin.CatalogRow{rotateRow()},
		UpsertCredentialErr: errors.New(canary),
	}
	srv := rotateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRotatePath, cookie, csrf, postgresRotateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeDependencyUnavailable || body.Error.Message != postgresRotateVaultUnsyncedMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if strings.Contains(rec.Body.String(), `"credential"`) || strings.Contains(rec.Body.String(), `"password"`) || strings.Contains(rec.Body.String(), canary) {
		t.Fatalf("503 leaked credential: %s", rec.Body.String())
	}
	if cat.AlterRolePasswordCalls != 1 {
		t.Fatalf("re-ALTER: %d", cat.AlterRolePasswordCalls)
	}
	var outcome, metadata string
	if err := srv.db.QueryRow(`SELECT outcome, metadata FROM audit_events WHERE action = 'postgres.credential.rotate' ORDER BY id DESC LIMIT 1`).Scan(&outcome, &metadata); err != nil {
		t.Fatal(err)
	}
	if outcome != "failure" {
		t.Fatalf("outcome = %s", outcome)
	}
	if strings.Contains(metadata, canary) {
		t.Fatalf("audit leaked canary: %s", metadata)
	}
}

func TestPostgresRotateAuditFailClosed(t *testing.T) {
	fx := loadPython49(t)
	srv := rotateServer(t, rotateMemory(t), secrets.DeriveVaultKey(fx.SessionSecret))
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
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRotatePath, cookie, csrf, postgresRotateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("audit failure returned credential: %s", rec.Body.String())
	}
}

func TestPostgresRotateWrongMethodsAre405(t *testing.T) {
	fx := loadPython49(t)
	cat := rotateMemory(t)
	srv := rotateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(method, postgresRotatePath, cookie, csrf, postgresRotateBody))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d %s", method, rec.Code, rec.Body.String())
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != CodeMethodNotAllowed {
			t.Fatalf("%s code = %q", method, body.Error.Code)
		}
	}
	post := httptest.NewRecorder()
	h.ServeHTTP(post, authed(http.MethodPost, postgresRotatePath, cookie, csrf, postgresRotateBody))
	if post.Code != http.StatusOK {
		t.Fatalf("POST status = %d %s", post.Code, post.Body.String())
	}
	alias := httptest.NewRecorder()
	h.ServeHTTP(alias, authed(http.MethodPost, "/api/v1/postgres/databases/project_a/rotate", cookie, csrf, postgresRotateBody))
	if alias.Code == http.StatusOK || alias.Code == http.StatusCreated {
		t.Fatal("must not register /rotate alias")
	}
	item := httptest.NewRecorder()
	h.ServeHTTP(item, authed(http.MethodPost, "/api/v1/postgres/databases/project_a", cookie, csrf, postgresRotateBody))
	if item.Code != http.StatusMethodNotAllowed {
		t.Fatalf("item POST status = %d %s", item.Code, item.Body.String())
	}
}

func TestPostgresRotateNilAdapterAuditsFailure(t *testing.T) {
	srv, _ := testServer(t, nil)
	srv.cfg.PostgresDatabase = "postgres"
	srv.cfg.PostgresUser = "redgres_console"
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRotatePath, cookie, csrf, postgresRotateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) {
		t.Fatalf("503 leaked credential: %s", rec.Body.String())
	}
	var outcome string
	if err := srv.db.QueryRow(`SELECT outcome FROM audit_events WHERE action = 'postgres.credential.rotate' ORDER BY id DESC LIMIT 1`).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome != "failure" {
		t.Fatalf("outcome = %s", outcome)
	}
}
