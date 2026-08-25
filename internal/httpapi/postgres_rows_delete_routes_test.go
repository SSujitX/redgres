package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/audit"
	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
)

const (
	postgresRowsPath         = "/api/v1/postgres/databases/project_a/tables/public/items/rows"
	postgresPrimaryKeyPath   = "/api/v1/postgres/databases/project_a/tables/public/items/primary-key"
	postgresRowsDeleteCanary = "wrong-canary-password"
)

func rowDeleteCatalog() *postgresadmin.MemoryCatalog {
	return &postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{
			{Name: "project_a", Owner: "project_a_role", AllowConn: true},
			{Name: "postgres", Owner: "postgres", AllowConn: true},
		},
		TableData: map[string]postgresadmin.MemoryTable{
			"project_a.public.items": {
				Columns:    []string{"id", "name"},
				Rows:       []map[string]any{{"id": 1, "name": "a"}},
				PrimaryKey: []string{"id"},
			},
			"project_a.public.no_pk": {
				Columns: []string{"name"},
			},
			"project_a.public.composite": {
				Columns:    []string{"org_id", "id"},
				PrimaryKey: []string{"org_id", "id"},
			},
		},
	}
}

func rowDeleteService(cat *postgresadmin.MemoryCatalog) *postgresadmin.Service {
	return postgresadmin.NewService(cat, postgresadmin.NewPolicy(config.Config{PostgresDatabase: "postgres"}))
}

func rowDeleteServer(t *testing.T, cat *postgresadmin.MemoryCatalog, enabled bool) *Server {
	t.Helper()
	srv := testServerWithPostgres(t, rowDeleteService(cat))
	srv.cfg.FeaturePostgresRowDelete = enabled
	return srv
}

func rowsDeleteJSON(table, password, values string) string {
	return `{"table_confirmation":"` + table + `","owner_password":"` + password + `","primary_key_values":` + values + `}`
}

func TestPostgresPrimaryKey200(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, false)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, postgresPrimaryKeyPath, cookie, "", ""))
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
	pk, _ := body["primary_key"].([]any)
	if len(pk) != 1 || pk[0] != "id" {
		t.Fatalf("primary_key = %#v", body["primary_key"])
	}
	id, _ := body["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("GET primary-key wrote audit: %d -> %d", before, after)
	}
}

func TestPostgresPrimaryKeyEmptyAndComposite(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, false)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	empty := httptest.NewRecorder()
	h.ServeHTTP(empty, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables/public/no_pk/primary-key", cookie, "", ""))
	if empty.Code != http.StatusOK {
		t.Fatalf("empty status = %d %s", empty.Code, empty.Body.String())
	}
	var emptyBody map[string]any
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatal(err)
	}
	pk, ok := emptyBody["primary_key"].([]any)
	if !ok || pk == nil || len(pk) != 0 {
		t.Fatalf("empty primary_key must be [] not null: %#v", emptyBody["primary_key"])
	}
	composite := httptest.NewRecorder()
	h.ServeHTTP(composite, authed(http.MethodGet, "/api/v1/postgres/databases/project_a/tables/public/composite/primary-key", cookie, "", ""))
	if composite.Code != http.StatusOK {
		t.Fatalf("composite status = %d %s", composite.Code, composite.Body.String())
	}
	var compositeBody map[string]any
	if err := json.Unmarshal(composite.Body.Bytes(), &compositeBody); err != nil {
		t.Fatal(err)
	}
	cols, _ := compositeBody["primary_key"].([]any)
	if len(cols) != 2 || cols[0] != "org_id" || cols[1] != "id" {
		t.Fatalf("composite = %#v", compositeBody["primary_key"])
	}
}

func TestPostgresPrimaryKey404(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, false)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	for _, path := range []string{
		"/api/v1/postgres/databases/project_a/tables/public/missing/primary-key",
		"/api/v1/postgres/databases/postgres/tables/public/items/primary-key",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, path, cookie, "", ""))
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), `"primary_key"`) {
			t.Fatalf("404 leaked primary_key: %s", rec.Body.String())
		}
	}
}

func TestPostgresRowsDeleteFlagOffBeforeDecode(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, false)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresRowsPath, cookie, csrf, `{"table_confirmation":"items","owner_password":"`+ownerPassword+`","primary_key_column":"id","primary_key_values":["1"]`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeForbidden || body.Error.Message != postgresRowsDeleteOffMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if cat.DeleteRowsCalls != 0 || cat.LastPrimaryKeyKey != "" {
		t.Fatalf("PostgreSQL on flag off: calls=%d pk=%q", cat.DeleteRowsCalls, cat.LastPrimaryKeyKey)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("flag off wrote audit: %d -> %d", before, after)
	}
}

func TestPostgresRowsDeleteRequiresCSRF(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresRowsPath, cookie, "", rowsDeleteJSON("items", ownerPassword, `["1"]`)))
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
	if cat.DeleteRowsCalls != 0 {
		t.Fatalf("DML before CSRF: %d", cat.DeleteRowsCalls)
	}
}

func TestPostgresRowsDeleteUnknownPrimaryKeyColumn(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresRowsPath, cookie, csrf, `{"table_confirmation":"items","owner_password":"`+ownerPassword+`","primary_key_column":"id","primary_key_values":["1"]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeValidationError || body.Error.Message != "Unknown field" {
		t.Fatalf("error = %#v", body.Error)
	}
	if cat.DeleteRowsCalls != 0 {
		t.Fatalf("DML on unknown field: %d", cat.DeleteRowsCalls)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("unknown field wrote audit: %d -> %d", before, after)
	}
}

func TestPostgresRowsDeleteConfirmationMismatchNoAudit(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	for _, body := range []string{
		rowsDeleteJSON("Items", ownerPassword, `["1"]`),
		rowsDeleteJSON("", ownerPassword, `["1"]`),
		rowsDeleteJSON("project_a", ownerPassword, `["1"]`),
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodDelete, postgresRowsPath, cookie, csrf, body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
		}
		var errBody errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
			t.Fatal(err)
		}
		if errBody.Error.Code != CodeValidationError || errBody.Error.Message != postgresRowsDeleteConfirmMessage {
			t.Fatalf("error = %#v", errBody.Error)
		}
		if errBody.Error.Fields["table_confirmation"] != "invalid" {
			t.Fatalf("fields = %#v", errBody.Error.Fields)
		}
	}
	if cat.DeleteRowsCalls != 0 || cat.LastPrimaryKeyKey != "" {
		t.Fatalf("PostgreSQL on confirmation fail: calls=%d pk=%q", cat.DeleteRowsCalls, cat.LastPrimaryKeyKey)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("confirmation wrote audit: %d -> %d", before, after)
	}
}

func TestPostgresRowsDeleteReauthRequiredMetadataDatabaseSchemaTable(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	attemptsBefore := countLoginAttempts(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresRowsPath, cookie, csrf, rowsDeleteJSON("items", postgresRowsDeleteCanary, `["1"]`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeReauthRequired || body.Error.Message != "Owner password is incorrect" {
		t.Fatalf("error = %#v", body.Error)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, postgresRowsDeleteCanary) || strings.Contains(raw, ownerPassword) {
		t.Fatalf("403 leaked password: %s", raw)
	}
	if cat.DeleteRowsCalls != 0 || cat.LastPrimaryKeyKey != "" {
		t.Fatalf("SQL before successful reauth: calls=%d pk=%q", cat.DeleteRowsCalls, cat.LastPrimaryKeyKey)
	}
	var metadata, action, outcome, target string
	if err := srv.db.QueryRow(`SELECT action, target, outcome, metadata FROM audit_events WHERE action = 'postgres.rows.delete' ORDER BY id DESC LIMIT 1`).Scan(&action, &target, &outcome, &metadata); err != nil {
		t.Fatal(err)
	}
	if action != "postgres.rows.delete" || outcome != "failure" || target != "project_a" {
		t.Fatalf("audit = %s/%s/%s", action, target, outcome)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "project_a" || meta["schema"] != "public" || meta["table"] != "items" {
		t.Fatalf("metadata = %s", metadata)
	}
	if len(meta) != 3 {
		t.Fatalf("metadata keys = %#v", meta)
	}
	if _, ok := meta["username"]; ok {
		t.Fatalf("reauth metadata must not use username: %s", metadata)
	}
	if strings.Contains(metadata, postgresRowsDeleteCanary) || strings.Contains(metadata, ownerPassword) || strings.Contains(metadata, "password") {
		t.Fatalf("audit leaked secret: %s", metadata)
	}
	if after := countLoginAttempts(t, srv); after != attemptsBefore+1 {
		t.Fatalf("reauth attempt was not persisted: %d -> %d", attemptsBefore, after)
	}
}

func TestPostgresRowsDeleteProtected404NoDML(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/postgres/databases/postgres/tables/public/items/rows", cookie, csrf, rowsDeleteJSON("items", ownerPassword, `["1"]`)))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "protected_resource") {
		t.Fatal("protected rows delete must be 404 not protected_resource")
	}
	if cat.DeleteRowsCalls != 0 || cat.LastPrimaryKeyKey != "" {
		t.Fatalf("DML on protected: calls=%d pk=%q", cat.DeleteRowsCalls, cat.LastPrimaryKeyKey)
	}
}

func TestPostgresRowsDeleteNoPKAndCompositeNoDML(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, table := range []string{"no_pk", "composite"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/postgres/databases/project_a/tables/public/"+table+"/rows", cookie, csrf, rowsDeleteJSON(table, ownerPassword, `["1"]`)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d %s", table, rec.Code, rec.Body.String())
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != CodeValidationError || body.Error.Fields["primary_key"] != "invalid" {
			t.Fatalf("%s error = %#v", table, body.Error)
		}
		if body.Error.Message != "This table does not have a single-column primary key." {
			t.Fatalf("%s message = %q", table, body.Error.Message)
		}
	}
	if cat.DeleteRowsCalls != 0 {
		t.Fatalf("DML without single-column PK: %d", cat.DeleteRowsCalls)
	}
}

func TestPostgresRowsDelete500Cap(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	over := make([]string, postgresadmin.MaxRowDeleteValues+1)
	for i := range over {
		over[i] = `"1"`
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresRowsPath, cookie, csrf, rowsDeleteJSON("items", ownerPassword, "["+strings.Join(over, ",")+"]")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("over cap status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Fields["primary_key_values"] != "invalid" || body.Error.Message != postgresRowsDeletePKValuesMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if cat.DeleteRowsCalls != 0 || cat.LastPrimaryKeyKey != "" {
		t.Fatalf("DML over cap: calls=%d pk=%q", cat.DeleteRowsCalls, cat.LastPrimaryKeyKey)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("over cap wrote audit: %d -> %d", before, after)
	}
	atCap := make([]string, postgresadmin.MaxRowDeleteValues)
	for i := range atCap {
		atCap[i] = `"1"`
	}
	okRec := httptest.NewRecorder()
	h.ServeHTTP(okRec, authed(http.MethodDelete, postgresRowsPath, cookie, csrf, rowsDeleteJSON("items", ownerPassword, "["+strings.Join(atCap, ",")+"]")))
	if okRec.Code != http.StatusOK {
		t.Fatalf("cap status = %d %s", okRec.Code, okRec.Body.String())
	}
	if cat.DeleteRowsCalls != 1 || len(cat.LastDeleteValues) != postgresadmin.MaxRowDeleteValues {
		t.Fatalf("cap DML = calls=%d n=%d", cat.DeleteRowsCalls, len(cat.LastDeleteValues))
	}
}

func TestPostgresRowsDeleteRejectsNullObjectArrayValues(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	for _, values := range []string{`[]`, `[null]`, `[{}]`, `[[]]`, `{"a":1}`} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodDelete, postgresRowsPath, cookie, csrf, rowsDeleteJSON("items", ownerPassword, values)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d %s", values, rec.Code, rec.Body.String())
		}
	}
	if cat.DeleteRowsCalls != 0 {
		t.Fatalf("DML on invalid values: %d", cat.DeleteRowsCalls)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("invalid values wrote audit: %d -> %d", before, after)
	}
}

func TestPostgresRowsDeleteCanaryPasswordAbsent(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresRowsPath, cookie, csrf, rowsDeleteJSON("items", postgresRowsDeleteCanary, `["1"]`)))
	raw := rec.Body.String()
	if strings.Contains(raw, postgresRowsDeleteCanary) || strings.Contains(raw, ownerPassword) {
		t.Fatalf("response leaked password: %s", raw)
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'postgres.rows.delete' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata, postgresRowsDeleteCanary) || strings.Contains(metadata, ownerPassword) {
		t.Fatalf("audit leaked password: %s", metadata)
	}
}

func TestPostgresRowsDelete200(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresRowsPath, cookie, csrf, rowsDeleteJSON("items", ownerPassword, `["1", 2, true]`)))
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
	if body["deleted"] != float64(3) {
		t.Fatalf("body = %#v", body)
	}
	id, _ := body["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
	if _, ok := body["message"]; ok {
		t.Fatalf("must not include sibling message: %#v", body)
	}
	if strings.Contains(rec.Body.String(), ownerPassword) {
		t.Fatalf("200 leaked password: %s", rec.Body.String())
	}
	if after := countAuditEvents(t, srv); after != before+1 {
		t.Fatalf("audit count %d -> %d", before, after)
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'postgres.rows.delete' AND outcome = 'success' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "project_a" || meta["schema"] != "public" || meta["table"] != "items" || meta["deleted"] != float64(3) {
		t.Fatalf("success metadata = %s", metadata)
	}
	if len(meta) != 4 {
		t.Fatalf("metadata keys = %#v", meta)
	}
}

func TestPostgresRowsDeleteAuditFailClosed(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	dead, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dead-audit-rows-delete.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dead.Close() })
	_ = dead.Close()
	srv.audit = audit.Store{DB: dead}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresRowsPath, cookie, csrf, rowsDeleteJSON("items", ownerPassword, `["1"]`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"deleted"`) {
		t.Fatalf("audit failure returned deleted: %s", rec.Body.String())
	}
	if cat.DeleteRowsCalls != 1 {
		t.Fatalf("DML must complete before audit fail: %d", cat.DeleteRowsCalls)
	}
}

func TestPostgresRowsDeleteRejectsPOST(t *testing.T) {
	cat := rowDeleteCatalog()
	srv := rowDeleteServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresRowsPath, cookie, csrf, `{}`))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if cat.DeleteRowsCalls != 0 {
		t.Fatalf("POST must not delete: %d", cat.DeleteRowsCalls)
	}
}
