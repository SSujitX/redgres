package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
)

func TestPostgresSecurityRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/postgres/security", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"summary", "databases", "connections", "saved_credential", "truncated"} {
		if _, ok := body[key]; ok {
			t.Fatalf("401 must not include %s: %s", key, rec.Body.String())
		}
	}
}

func TestPostgresSecurityUnavailableWithoutAdapter(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/security", cookie, csrf, ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dependency_unavailable") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"summary"`) || strings.Contains(rec.Body.String(), `"databases"`) {
		t.Fatal("unavailable security must not look healthy")
	}
}

func TestPostgresSecurityRejectsOtherMethods(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(method, "/api/v1/postgres/security", cookie, csrf, `{}`))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d %s", method, rec.Code, rec.Body.String())
		}
	}
}

func TestPostgresSecurityReturnsClusterOverview(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{
			{Name: "postgres", Owner: "postgres", AllowConn: true, ConnectionCount: 1, OwnerIsSuperuser: true, OwnerCanLogin: true},
			{Name: "database_console_vault", Owner: "postgres", AllowConn: true},
			{Name: "template0", Owner: "postgres", AllowConn: false, IsTemplate: true},
			{Name: "project_a", Owner: "project_a_role", AllowConn: true, PublicCanConnect: true, ConnectionCount: 2, OwnerCanLogin: true},
		},
		Connections: []postgresadmin.ConnectionGroup{
			{Database: "postgres", User: "postgres", Client: "local", Application: "redgres", State: "idle", Count: 1},
			{Database: "project_a", User: "project_a_role", Client: "10.0.0.1", Application: "app", State: "active", Count: 2},
		},
	}, postgresadmin.NewPolicy(config.Config{PostgresDatabase: "postgres", PostgresUser: "redgres_console"}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/security", cookie, csrf, ""))
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
	if _, ok := body["request_id"].(string); !ok {
		t.Fatal("missing request_id")
	}
	if body["truncated"] != false {
		t.Fatalf("truncated = %#v", body["truncated"])
	}
	cred, _ := body["saved_credential"].(map[string]any)
	if cred["status"] != "not_available" || cred["reason"] != "vault_not_implemented" {
		t.Fatalf("saved_credential = %#v", body["saved_credential"])
	}
	summary, _ := body["summary"].(map[string]any)
	if summary["database_count"] != float64(3) || summary["public_connect_count"] != float64(1) || summary["active_connection_count"] != float64(3) || summary["connection_group_count"] != float64(2) {
		t.Fatalf("summary = %#v", summary)
	}
	if _, ok := summary["missing_password_count"]; ok {
		t.Fatal("missing_password_count must be absent")
	}
	dbs, _ := body["databases"].([]any)
	if dbs == nil || len(dbs) != 3 {
		t.Fatalf("databases = %#v", body["databases"])
	}
	first, _ := dbs[0].(map[string]any)
	if first["name"] != "database_console_vault" || first["protected"] != true {
		t.Fatalf("first = %#v", first)
	}
	if _, ok := first["database"]; ok {
		t.Fatal("JSON key must be name, not database")
	}
	if _, ok := first["size"]; ok || first["can_rotate"] != nil || first["has_saved_password"] != nil {
		t.Fatalf("forbidden database keys = %#v", first)
	}
	conns, _ := body["connections"].([]any)
	if conns == nil || len(conns) != 2 {
		t.Fatalf("connections = %#v", body["connections"])
	}
	for _, leak := range []string{"canary", "secret", "postgresql://", "query", "datacl"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("leaked %q in %s", leak, rec.Body.String())
		}
	}
}

func TestPostgresSecurityCapsTruncatedFlag(t *testing.T) {
	rows := make([]postgresadmin.CatalogRow, 501)
	for i := range rows {
		rows[i] = postgresadmin.CatalogRow{Name: fmt.Sprintf("db%03d", i), Owner: "role", AllowConn: true}
	}
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Rows: rows}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/security", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["truncated"] != true {
		t.Fatalf("truncated = %#v", body["truncated"])
	}
	dbs, _ := body["databases"].([]any)
	conns, _ := body["connections"].([]any)
	if len(dbs) != 500 || conns == nil || len(conns) != 0 {
		t.Fatalf("arrays = dbs=%d conns=%#v", len(dbs), body["connections"])
	}
	summary, _ := body["summary"].(map[string]any)
	if summary["database_count"] != float64(501) {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPostgresSecurityCatalogErrorIs503WithoutCanary(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Err: errors.New("postgresql://canary:secret@127.0.0.1/db")}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/security", cookie, csrf, ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dependency_unavailable") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "canary") || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("leaked %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"summary"`) {
		t.Fatal("503 must not include summary")
	}
}

func TestPostgresSecurityDoesNotExposeProtectedOnListOrDetails(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "postgres", Owner: "postgres", AllowConn: true},
		{Name: "database_console_vault", Owner: "postgres", AllowConn: true},
		{Name: "project_a", Owner: "project_a_role", AllowConn: true},
	}}, postgresadmin.NewPolicy(config.Config{PostgresDatabase: "postgres"}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, name := range []string{"postgres", "database_console_vault"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases/"+name, cookie, csrf, ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s details status = %d %s", name, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/postgres/databases", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "postgres") || strings.Contains(rec.Body.String(), "database_console_vault") {
		t.Fatalf("list leaked protected names: %s", rec.Body.String())
	}
}
