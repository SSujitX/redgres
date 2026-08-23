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
	svc := postgresadmin.NewService(postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
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
	svc := postgresadmin.NewService(postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
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

func TestPostgresDetailsRejectsInvalidName(t *testing.T) {
	srv := testServerWithPostgres(t, postgresadmin.NewService(postgresadmin.MemoryCatalog{}, postgresadmin.NewPolicy(config.Config{})))
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

func TestPostgresListCanaryErrorIsRedacted(t *testing.T) {
	svc := postgresadmin.NewService(postgresadmin.MemoryCatalog{Err: errors.New("postgresql://canary:secret@127.0.0.1/db")}, postgresadmin.NewPolicy(config.Config{}))
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
