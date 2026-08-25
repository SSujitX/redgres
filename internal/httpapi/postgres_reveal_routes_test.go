package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/audit"
	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/SSujitX/redgres/internal/secrets"
)

const postgresRevealPath = "/api/v1/postgres/databases/project_a/connection/reveal"

type python49Fixtures struct {
	SessionSecret string `json:"session_secret"`
	ASCII         struct {
		Plaintext string `json:"plaintext"`
		Token     string `json:"token"`
	} `json:"ascii"`
}

func loadPython49(t *testing.T) python49Fixtures {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "secrets", "testdata", "python49.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx python49Fixtures
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if fx.ASCII.Plaintext == "" || fx.ASCII.Token == "" {
		t.Fatal("python49 fixture missing ASCII canary")
	}
	return fx
}

func revealMemory(t *testing.T, fx python49Fixtures) *postgresadmin.MemoryCatalog {
	t.Helper()
	return &postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{{Name: "project_a", Owner: "project_a_role", AllowConn: true}},
		Ciphertexts: map[string]string{
			"project_a_role": fx.ASCII.Token,
		},
		SavedRoles: []string{"project_a_role"},
	}
}

func revealInventory(t *testing.T, cat *postgresadmin.MemoryCatalog, fx python49Fixtures) postgresadmin.Inventory {
	t.Helper()
	return postgresadmin.NewServiceWithVaultKey(cat, postgresadmin.NewPolicy(config.Config{
		PostgresDatabase: "postgres",
		PostgresUser:     "redgres_console",
	}), secrets.DeriveVaultKey(fx.SessionSecret))
}

func TestPostgresRevealRequiresSession(t *testing.T) {
	fx := loadPython49(t)
	srv := testServerWithPostgres(t, revealInventory(t, revealMemory(t, fx), fx))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, postgresRevealPath, strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeUnauthorized {
		t.Fatalf("code = %q", body.Error.Code)
	}
	if strings.Contains(rec.Body.String(), `"credential"`) || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("401 leaked credential: %s", rec.Body.String())
	}
}

func TestPostgresRevealRequiresCSRF(t *testing.T) {
	fx := loadPython49(t)
	srv := testServerWithPostgres(t, revealInventory(t, revealMemory(t, fx), fx))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRevealPath, cookie, "", `{}`))
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

func TestPostgresRevealCapabilityIsCredentials(t *testing.T) {
	if !hasCapability("postgres.credentials") {
		t.Fatal("postgres.credentials must be granted")
	}
	srv, _ := testServer(t, nil)
	reached := false
	handler := srv.requireCapability("postgres.export")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, postgresRevealPath, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
	if reached {
		t.Fatal("handler ran without the capability")
	}
}

func TestPostgresRevealRejectsInvalidNameWithoutAudit(t *testing.T) {
	fx := loadPython49(t)
	cat := revealMemory(t, fx)
	srv := testServerWithPostgres(t, revealInventory(t, cat, fx))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/postgres/databases/bad-name/connection/reveal", cookie, csrf, `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeValidationError {
		t.Fatalf("code = %q", body.Error.Code)
	}
	if strings.Contains(rec.Body.String(), "bad-name") {
		t.Fatalf("raw param echoed: %s", rec.Body.String())
	}
	if len(cat.EncryptedPasswordCalls) != 0 {
		t.Fatalf("invalid name must not SELECT: %#v", cat.EncryptedPasswordCalls)
	}
	var n int
	if err := srv.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action = 'postgres.credential.reveal'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("invalid name must not audit: %d", n)
	}
}

func TestPostgresRevealProtectedAndMissingAreNotFound(t *testing.T) {
	fx := loadPython49(t)
	cat := revealMemory(t, fx)
	srv := testServerWithPostgres(t, revealInventory(t, cat, fx))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, name := range []string{"postgres", "template0", "template1", "database_console_vault", "missing_db"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/postgres/databases/"+name+"/connection/reveal", cookie, csrf, `{}`))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d %s", name, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"not_found"`) {
			t.Fatalf("%s body = %s", name, rec.Body.String())
		}
	}
	if len(cat.EncryptedPasswordCalls) != 0 {
		t.Fatalf("protected must not SELECT: %#v", cat.EncryptedPasswordCalls)
	}
}

func TestPostgresRevealVaultCanaryIs503(t *testing.T) {
	fx := loadPython49(t)
	cat := revealMemory(t, fx)
	cat.CiphertextErr = errors.New("postgresql://canary-secret@10.0.0.1/db")
	srv := testServerWithPostgres(t, revealInventory(t, cat, fx))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRevealPath, cookie, csrf, `{}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PostgreSQL is unavailable") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "canary-secret") || strings.Contains(rec.Body.String(), fx.ASCII.Plaintext) || strings.Contains(rec.Body.String(), fx.ASCII.Token) {
		t.Fatalf("leaked canary: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) {
		t.Fatalf("503 leaked credential: %s", rec.Body.String())
	}
}

func TestPostgresRevealSuccessReturnsPasswordAndURLs(t *testing.T) {
	fx := loadPython49(t)
	cat := revealMemory(t, fx)
	srv := testServerWithPostgres(t, revealInventory(t, cat, fx))
	srv.cfg.PostgresPublicHost = "db.example.com"
	srv.cfg.PostgresDirectPort = "5432"
	srv.cfg.PostgresPooledPort = "6432"
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRevealPath, cookie, csrf, `{"owner_password":"wrong-canary","password":"wrong-canary"}`))
	if rec.Code != http.StatusOK {
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
	if cred["username"] != "project_a_role" {
		t.Fatalf("username = %#v", cred["username"])
	}
	if cred["password"] != fx.ASCII.Plaintext {
		t.Fatal("password mismatch")
	}
	if cred["one_time"] != false {
		t.Fatalf("one_time = %#v", cred["one_time"])
	}
	urls, _ := cred["urls"].(map[string]any)
	if urls["direct"] != "postgresql://project_a_role:"+fx.ASCII.Plaintext+"@db.example.com:5432/project_a?sslmode=require" {
		t.Fatalf("direct = %#v", urls["direct"])
	}
	if urls["pooled"] != "postgresql://project_a_role:"+fx.ASCII.Plaintext+"@db.example.com:6432/project_a?sslmode=require" {
		t.Fatalf("pooled = %#v", urls["pooled"])
	}
	id, _ := body["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
	if strings.Contains(rec.Body.String(), "wrong-canary") {
		t.Fatalf("treated body as owner_password: %s", rec.Body.String())
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'postgres.credential.reveal' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "project_a" || meta["owner"] != "project_a_role" {
		t.Fatalf("metadata = %s", metadata)
	}
	if len(meta) != 2 {
		t.Fatalf("metadata keys = %#v", meta)
	}
	if strings.Contains(metadata, fx.ASCII.Plaintext) || strings.Contains(metadata, "postgresql://") || strings.Contains(metadata, "password") {
		t.Fatalf("audit leaked secret: %s", metadata)
	}
}

func TestPostgresRevealOmitsURLsWhenPublicHostUnset(t *testing.T) {
	fx := loadPython49(t)
	srv := testServerWithPostgres(t, revealInventory(t, revealMemory(t, fx), fx))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRevealPath, cookie, csrf, ``))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	cred, _ := body["credential"].(map[string]any)
	if cred["password"] != fx.ASCII.Plaintext {
		t.Fatal("password must still be returned")
	}
	if _, ok := cred["urls"]; ok {
		t.Fatalf("urls must be omitted: %#v", cred)
	}
}

func TestPostgresRevealAuditFailClosed(t *testing.T) {
	fx := loadPython49(t)
	srv := testServerWithPostgres(t, revealInventory(t, revealMemory(t, fx), fx))
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
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRevealPath, cookie, csrf, `{}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) || strings.Contains(rec.Body.String(), fx.ASCII.Plaintext) || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("audit failure returned credential: %s", rec.Body.String())
	}
}

func TestPostgresRevealGETIsMethodNotAllowed(t *testing.T) {
	fx := loadPython49(t)
	srv := testServerWithPostgres(t, revealInventory(t, revealMemory(t, fx), fx))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, postgresRevealPath, cookie, csrf, ""))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "method_not_allowed") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPostgresConnectionGETStillDoesNotDecrypt(t *testing.T) {
	fx := loadPython49(t)
	cat := revealMemory(t, fx)
	srv := testServerWithPostgres(t, revealInventory(t, cat, fx))
	srv.cfg.PostgresPublicHost = "db.example.com"
	srv.cfg.PostgresDirectPort = "5432"
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, postgresConnectionPath, cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if len(cat.EncryptedPasswordCalls) != 0 {
		t.Fatalf("GET connection must not SELECT ciphertext: %#v", cat.EncryptedPasswordCalls)
	}
	if strings.Contains(rec.Body.String(), fx.ASCII.Plaintext) || strings.Contains(rec.Body.String(), fx.ASCII.Token) {
		t.Fatalf("GET leaked plaintext or token: %s", rec.Body.String())
	}
	body := decodeConnectionBody(t, rec)
	if _, ok := body["password"]; ok {
		t.Fatal("GET must not include password")
	}
}
