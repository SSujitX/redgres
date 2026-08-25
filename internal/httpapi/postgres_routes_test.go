package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
)

func testServerWithPostgres(t *testing.T, inventory postgresadmin.Inventory) *Server {
	t.Helper()
	srv, _ := testServer(t, nil)
	srv.postgres = inventory
	return srv
}

func TestPostgresListRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/postgres/databases", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestPostgresListUnavailableWithoutAdapter(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases", cookie, csrf, ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dependency_unavailable") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"databases"`) {
		t.Fatal("unavailable list must not look healthy")
	}
}

func TestPostgresListReturnsManageableOnly(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "postgres", Owner: "postgres", AllowConn: true},
		{Name: "project_a", Owner: "project_a_role", AllowConn: true},
	}}, postgresadmin.NewPolicy(config.Config{PostgresDatabase: "postgres", PostgresUser: "redgres_console"}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	dbs, _ := body["databases"].([]any)
	if len(dbs) != 1 {
		t.Fatalf("databases = %#v", body["databases"])
	}
	row, _ := dbs[0].(map[string]any)
	if row["name"] != "project_a" {
		t.Fatalf("row = %#v", row)
	}
	if _, ok := body["request_id"].(string); !ok {
		t.Fatal("missing request_id")
	}
}

func TestPostgresDetailsProtectedIsNotFound(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "project_a", Owner: "project_a_role", AllowConn: true, SizePretty: "1 MB"},
	}}, postgresadmin.NewPolicy(config.Config{PostgresDatabase: "postgres", PostgresProtectedDatabases: []string{"ops_extra"}}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, name := range []string{"postgres", "template0", "template1", "database_console_vault", "ops_extra", "missing_db"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/"+name, cookie, csrf, ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d %s", name, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"not_found"`) {
			t.Fatalf("%s body = %s", name, rec.Body.String())
		}
	}
}

func TestPostgresDetailsSavedCredentialPresent(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{
		Rows:       []postgresadmin.CatalogRow{{Name: "project_a", Owner: "project_a_role", AllowConn: true, SizePretty: "1 MB"}},
		SavedRoles: []string{"project_a_role"},
	}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	db, _ := body["database"].(map[string]any)
	cred, _ := db["saved_credential"].(map[string]any)
	if cred["status"] != "present" || cred["reason"] != "" {
		t.Fatalf("saved_credential = %#v", db["saved_credential"])
	}
	if strings.Contains(rec.Body.String(), "vault_not_implemented") || strings.Contains(rec.Body.String(), "encrypted_password") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPostgresDetailsSavedCredentialMissing(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{{Name: "project_a", Owner: "project_a_role", AllowConn: true, SizePretty: "1 MB"}},
	}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	db, _ := body["database"].(map[string]any)
	cred, _ := db["saved_credential"].(map[string]any)
	if cred["status"] != "missing" || cred["reason"] != "" {
		t.Fatalf("saved_credential = %#v", db["saved_credential"])
	}
}

func TestPostgresDetailsSavedCredentialVaultUnavailable(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{
		Rows:     []postgresadmin.CatalogRow{{Name: "project_a", Owner: "project_a_role", AllowConn: true, SizePretty: "1 MB"}},
		VaultErr: errors.New("postgresql://canary:secret@127.0.0.1/db"),
	}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	db, _ := body["database"].(map[string]any)
	cred, _ := db["saved_credential"].(map[string]any)
	if cred["status"] != "not_available" || cred["reason"] != "vault_unavailable" {
		t.Fatalf("saved_credential = %#v", db["saved_credential"])
	}
	for _, leak := range []string{"canary", "secret", "postgresql://", "vault_not_implemented"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("leaked %q in %s", leak, rec.Body.String())
		}
	}
}

func TestPostgresDetailsRejectsInvalidName(t *testing.T) {
	srv := testServerWithPostgres(t, postgresadmin.NewService(&postgresadmin.MemoryCatalog{}, postgresadmin.NewPolicy(config.Config{})))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/bad-name", cookie, csrf, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPostgresListRejectsPOST(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/postgres/databases", cookie, csrf, `{}`))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPostgresTablesRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/postgres/databases/project_a/tables", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestPostgresTablesUnavailableWithoutAdapter(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables", cookie, csrf, ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dependency_unavailable") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"tables"`) {
		t.Fatal("unavailable tables must not look healthy")
	}
}

func TestPostgresTablesReturnsManageableOnly(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{
			{Name: "project_a", Owner: "project_a_role", AllowConn: true},
		},
		Tables: map[string][]postgresadmin.TableItem{
			"project_a": {{Schema: "public", Name: "items"}},
		},
	}, postgresadmin.NewPolicy(config.Config{PostgresDatabase: "postgres"}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	tables, _ := body["tables"].([]any)
	if len(tables) != 1 {
		t.Fatalf("tables = %#v", body["tables"])
	}
	row, _ := tables[0].(map[string]any)
	if row["schema"] != "public" || row["name"] != "items" {
		t.Fatalf("row = %#v", row)
	}
	if body["truncated"] != false {
		t.Fatalf("truncated = %#v", body["truncated"])
	}
}

func TestPostgresTablesEmptyIsHealthy(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{
			{Name: "project_a", Owner: "project_a_role", AllowConn: true},
		},
	}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	tables, ok := body["tables"].([]any)
	if !ok || len(tables) != 0 {
		t.Fatalf("empty tables = %#v", body["tables"])
	}
}

func TestPostgresTablesProtectedIsNotFound(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "project_a", Owner: "project_a_role", AllowConn: true},
	}}, postgresadmin.NewPolicy(config.Config{PostgresDatabase: "postgres", PostgresProtectedDatabases: []string{"ops_extra"}}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, name := range []string{"postgres", "template0", "template1", "database_console_vault", "ops_extra", "missing_db"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/"+name+"/tables", cookie, csrf, ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d %s", name, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"not_found"`) {
			t.Fatalf("%s body = %s", name, rec.Body.String())
		}
	}
}

func TestPostgresTablesRejectsInvalidName(t *testing.T) {
	srv := testServerWithPostgres(t, postgresadmin.NewService(&postgresadmin.MemoryCatalog{}, postgresadmin.NewPolicy(config.Config{})))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/bad-name/tables", cookie, csrf, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPostgresTablesRejectsPOST(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/postgres/databases/project_a/tables", cookie, csrf, `{}`))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestPostgresTablesCanaryErrorIsRedacted(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{
			{Name: "project_a", Owner: "project_a_role", AllowConn: true},
		},
		TablesErr: errors.New("postgresql://canary:secret@127.0.0.1/db"),
	}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables", cookie, csrf, ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "canary") || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("leaked %s", rec.Body.String())
	}
}

func TestPostgresListCanaryErrorIsRedacted(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Err: errors.New("postgresql://canary:secret@127.0.0.1/db")}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases", cookie, csrf, ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "canary") || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("leaked %s", rec.Body.String())
	}
}

func rowService(t *testing.T) *postgresadmin.Service {
	t.Helper()
	return postgresadmin.NewService(&postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{{Name: "project_a", Owner: "project_a_role", AllowConn: true}},
		TableData: map[string]postgresadmin.MemoryTable{
			"project_a.public.items": {
				Columns: []string{"id", "name"},
				Rows:    []map[string]any{{"id": 1, "name": "a"}},
			},
		},
	}, postgresadmin.NewPolicy(config.Config{PostgresDatabase: "postgres"}))
}

func TestPostgresRowsRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/postgres/databases/project_a/tables/public/items/rows", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestPostgresRowsUnavailableWithoutAdapter(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables/public/items/rows", cookie, csrf, ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"rows"`) {
		t.Fatal("unavailable rows must not look healthy")
	}
}

func TestPostgresRowsReturnsPage(t *testing.T) {
	srv := testServerWithPostgres(t, rowService(t))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables/public/items/rows?limit=501&offset=-1", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["limit"] != float64(50) || body["offset"] != float64(0) || body["total"] != float64(1) {
		t.Fatalf("page = %#v", body)
	}
	rows, _ := body["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", body["rows"])
	}
}

func TestPostgresRowsProtectedDatabaseIsNotFound(t *testing.T) {
	srv := testServerWithPostgres(t, rowService(t))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/postgres/tables/public/items/rows", cookie, csrf, ""))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestPostgresRowsRejectsInvalidNames(t *testing.T) {
	srv := testServerWithPostgres(t, rowService(t))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	cases := []struct {
		path string
		want string
	}{
		{"/api/v1/postgres/databases/bad-name/tables/public/items/rows", "Invalid database name"},
		{"/api/v1/postgres/databases/project_a/tables/bad-name/items/rows", "Invalid schema name"},
		{"/api/v1/postgres/databases/project_a/tables/public/bad-name/rows", "Invalid table name"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, tc.path, cookie, csrf, ""))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s: %d %s", tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestPostgresRowsRejectsLongQuery(t *testing.T) {
	srv := testServerWithPostgres(t, rowService(t))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	q := strings.Repeat("x", 129)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables/public/items/rows?q="+q, cookie, csrf, ""))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"q"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestPostgresRowsRejectsBadLimit(t *testing.T) {
	srv := testServerWithPostgres(t, rowService(t))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables/public/items/rows?limit=abc", cookie, csrf, ""))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"limit"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestPostgresRowsRejectsPOST(t *testing.T) {
	srv := testServerWithPostgres(t, rowService(t))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/postgres/databases/project_a/tables/public/items/rows", cookie, csrf, `{}`))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestPostgresRowsMissingTableIsNotFound(t *testing.T) {
	srv := testServerWithPostgres(t, rowService(t))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables/public/missing/rows", cookie, csrf, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPostgresRowsCanaryErrorIsRedacted(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{
		Rows:    []postgresadmin.CatalogRow{{Name: "project_a", Owner: "project_a_role", AllowConn: true}},
		RowsErr: errors.New("postgresql://canary:secret@127.0.0.1/db"),
	}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables/public/items/rows", cookie, csrf, ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "canary") || strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), `"rows"`) {
		t.Fatalf("leaked %s", rec.Body.String())
	}
}
