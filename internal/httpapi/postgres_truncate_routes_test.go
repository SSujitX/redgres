package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/audit"
	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
)

const (
	postgresTruncatePath   = "/api/v1/postgres/databases/project_a/truncate"
	postgresTruncateCanary = "wrong-canary-password"
)

func truncateCatalog() *postgresadmin.MemoryCatalog {
	return &postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{
			{Name: "project_a", Owner: "project_a_role", AllowConn: true},
			{Name: "postgres", Owner: "postgres", AllowConn: true},
		},
		Tables: map[string][]postgresadmin.TableItem{
			"project_a": {
				{Schema: "public", Name: "items"},
				{Schema: "other", Name: "t2"},
			},
		},
	}
}

func truncateService(cat *postgresadmin.MemoryCatalog) *postgresadmin.Service {
	return postgresadmin.NewService(cat, postgresadmin.NewPolicy(config.Config{PostgresDatabase: "postgres"}))
}

func truncateServer(t *testing.T, cat *postgresadmin.MemoryCatalog, enabled bool) *Server {
	t.Helper()
	srv := testServerWithPostgres(t, truncateService(cat))
	srv.cfg.FeaturePostgresTruncate = enabled
	return srv
}

func truncateJSON(database, password string) string {
	return `{"database_confirmation":"` + database + `","owner_password":"` + password + `"}`
}

func TestPostgresTruncateFlagOffBeforeDecode(t *testing.T) {
	cat := truncateCatalog()
	srv := truncateServer(t, cat, false)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, `{"database_confirmation":"project_a","owner_password":"`+ownerPassword+`","cascade":true`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeForbidden || body.Error.Message != postgresTruncateOffMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	if cat.TruncateCalls != 0 || cat.LastTablesDB != "" {
		t.Fatalf("PostgreSQL on flag off: calls=%d tables=%q", cat.TruncateCalls, cat.LastTablesDB)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("flag off wrote audit: %d -> %d", before, after)
	}
}

func TestPostgresTruncateRequiresCSRF(t *testing.T) {
	cat := truncateCatalog()
	srv := truncateServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresTruncatePath, cookie, "", truncateJSON("project_a", ownerPassword)))
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
	if cat.TruncateCalls != 0 {
		t.Fatalf("SQL before CSRF: %d", cat.TruncateCalls)
	}
}

func TestPostgresTruncateUnknownFields(t *testing.T) {
	cat := truncateCatalog()
	srv := truncateServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	for _, extra := range []string{
		`"confirmation":"project_a"`,
		`"table_confirmation":"items"`,
		`"password":"x"`,
		`"tables":["public.items"]`,
		`"cascade":true`,
	} {
		rec := httptest.NewRecorder()
		body := `{"database_confirmation":"project_a","owner_password":"` + ownerPassword + `",` + extra + `}`
		h.ServeHTTP(rec, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d %s", extra, rec.Code, rec.Body.String())
		}
		var errBody errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
			t.Fatal(err)
		}
		if errBody.Error.Code != CodeValidationError || errBody.Error.Message != "Unknown field" {
			t.Fatalf("%s error = %#v", extra, errBody.Error)
		}
	}
	if cat.TruncateCalls != 0 {
		t.Fatalf("SQL on unknown field: %d", cat.TruncateCalls)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("unknown field wrote audit: %d -> %d", before, after)
	}
}

func TestPostgresTruncateConfirmationMismatchNoAudit(t *testing.T) {
	cat := truncateCatalog()
	srv := truncateServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	for _, body := range []string{
		truncateJSON("Project_a", ownerPassword),
		truncateJSON("", ownerPassword),
		truncateJSON("project_b", ownerPassword),
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
		}
		var errBody errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
			t.Fatal(err)
		}
		if errBody.Error.Code != CodeValidationError || errBody.Error.Message != postgresTruncateConfirmMessage {
			t.Fatalf("error = %#v", errBody.Error)
		}
		if errBody.Error.Fields["database_confirmation"] != "invalid" {
			t.Fatalf("fields = %#v", errBody.Error.Fields)
		}
	}
	if cat.TruncateCalls != 0 || cat.LastTablesDB != "" {
		t.Fatalf("PostgreSQL on confirmation fail: calls=%d tables=%q", cat.TruncateCalls, cat.LastTablesDB)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("confirmation wrote audit: %d -> %d", before, after)
	}
}

func TestPostgresTruncateReauthRequiredMetadataDatabaseOnly(t *testing.T) {
	cat := truncateCatalog()
	srv := truncateServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	attemptsBefore := countLoginAttempts(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, truncateJSON("project_a", postgresTruncateCanary)))
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
	if strings.Contains(raw, postgresTruncateCanary) || strings.Contains(raw, ownerPassword) {
		t.Fatalf("403 leaked password: %s", raw)
	}
	if rec.Code == http.StatusTooManyRequests {
		t.Fatal("reauth must not return 429")
	}
	if cat.TruncateCalls != 0 || cat.LastTablesDB != "" {
		t.Fatalf("SQL before successful reauth: calls=%d tables=%q", cat.TruncateCalls, cat.LastTablesDB)
	}
	var metadata, action, outcome, target string
	if err := srv.db.QueryRow(`SELECT action, target, outcome, metadata FROM audit_events WHERE action = 'postgres.database.truncate' ORDER BY id DESC LIMIT 1`).Scan(&action, &target, &outcome, &metadata); err != nil {
		t.Fatal(err)
	}
	if action != "postgres.database.truncate" || outcome != "failure" || target != "project_a" {
		t.Fatalf("audit = %s/%s/%s", action, target, outcome)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "project_a" || len(meta) != 1 {
		t.Fatalf("metadata = %s", metadata)
	}
	if strings.Contains(metadata, postgresTruncateCanary) || strings.Contains(metadata, ownerPassword) || strings.Contains(metadata, "password") {
		t.Fatalf("audit leaked secret: %s", metadata)
	}
	if after := countLoginAttempts(t, srv); after != attemptsBefore {
		t.Fatalf("login_attempts changed %d -> %d", attemptsBefore, after)
	}
}

func TestPostgresTruncateProtected404NoSQL(t *testing.T) {
	cat := truncateCatalog()
	srv := truncateServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/postgres/databases/postgres/truncate", cookie, csrf, truncateJSON("postgres", ownerPassword)))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "protected_resource") {
		t.Fatal("protected truncate must be 404 not protected_resource")
	}
	if cat.TruncateCalls != 0 {
		t.Fatalf("SQL on protected: %d", cat.TruncateCalls)
	}
}

func TestPostgresTruncateTableList409(t *testing.T) {
	cat := truncateCatalog()
	items := make([]postgresadmin.TableItem, 501)
	for i := range items {
		items[i] = postgresadmin.TableItem{Schema: "public", Name: "t" + strconv.Itoa(i)}
	}
	cat.Tables["project_a"] = items
	srv := truncateServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, truncateJSON("project_a", ownerPassword)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeConflict || body.Error.Message != postgresTruncateTableListMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if cat.TruncateCalls != 0 {
		t.Fatalf("SQL on truncated list: %d", cat.TruncateCalls)
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'postgres.database.truncate' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "project_a" || len(meta) != 1 {
		t.Fatalf("metadata = %s", metadata)
	}
}

func TestPostgresTruncateInProgress409(t *testing.T) {
	cat := &postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{
			{Name: "project_a", Owner: "project_a_role", AllowConn: true},
		},
		Tables: map[string][]postgresadmin.TableItem{
			"project_a": {{Schema: "public", Name: "items"}},
		},
		TruncateStarted: make(chan struct{}, 1),
		TruncateHold:    make(chan struct{}),
	}
	srv := truncateServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, truncateJSON("project_a", ownerPassword)))
		done <- rec
	}()
	select {
	case <-cat.TruncateStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first truncate did not reach SQL")
	}
	second := httptest.NewRecorder()
	h.ServeHTTP(second, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, truncateJSON("project_a", ownerPassword)))
	if second.Code != http.StatusConflict {
		t.Fatalf("second status = %d %s", second.Code, second.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(second.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeOperationInProgress || body.Error.Message != postgresTruncateInProgressMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if body.Error.Message == postgresRotateInProgressMessage || body.Error.Message == postgresDuplicateInProgressMessage {
		t.Fatal("must not reuse rotate/duplicate 409 copy")
	}
	close(cat.TruncateHold)
	select {
	case rec := <-done:
		if rec.Code != http.StatusOK {
			t.Fatalf("first status = %d %s", rec.Code, rec.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first truncate blocked")
	}
}

func TestPostgresTruncateEmptyTables200(t *testing.T) {
	cat := truncateCatalog()
	cat.Tables["project_a"] = nil
	srv := truncateServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, truncateJSON("project_a", ownerPassword)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	failed, ok := body["failed"].([]any)
	if !ok || failed == nil {
		t.Fatalf("failed must be [] not null: %#v", body["failed"])
	}
	if body["truncated"] != float64(0) || body["total_tables"] != float64(0) || len(failed) != 0 {
		t.Fatalf("body = %#v", body)
	}
	if cat.TruncateCalls != 0 {
		t.Fatalf("SQL on empty: %d", cat.TruncateCalls)
	}
}

func TestPostgresTruncate200ShapeAndAudit(t *testing.T) {
	cat := truncateCatalog()
	srv := truncateServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, truncateJSON("project_a", ownerPassword)))
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
	failed, ok := body["failed"].([]any)
	if !ok || failed == nil {
		t.Fatalf("failed must be [] not null: %#v", body["failed"])
	}
	if body["truncated"] != float64(2) || body["total_tables"] != float64(2) || len(failed) != 0 {
		t.Fatalf("body = %#v", body)
	}
	id, _ := body["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
	if _, present := body["message"]; present {
		t.Fatalf("must not include sibling message: %#v", body)
	}
	if truncated, ok := body["truncated"].(bool); ok {
		t.Fatalf("truncated must be a count, not %v", truncated)
	}
	if strings.Contains(rec.Body.String(), ownerPassword) {
		t.Fatalf("200 leaked password: %s", rec.Body.String())
	}
	if cat.TruncateCalls != 1 {
		t.Fatalf("SQL calls = %d", cat.TruncateCalls)
	}
	if after := countAuditEvents(t, srv); after != before+1 {
		t.Fatalf("audit count %d -> %d", before, after)
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'postgres.database.truncate' AND outcome = 'success' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "project_a" || meta["truncated"] != float64(2) || meta["total_tables"] != float64(2) {
		t.Fatalf("success metadata = %s", metadata)
	}
	if len(meta) != 3 {
		t.Fatalf("metadata keys = %#v", meta)
	}
	if strings.Contains(metadata, ownerPassword) {
		t.Fatalf("audit leaked password: %s", metadata)
	}
}

func TestPostgresTruncateAuditFailClosed(t *testing.T) {
	cat := truncateCatalog()
	srv := truncateServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	dead, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dead-audit-truncate.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dead.Close() })
	_ = dead.Close()
	srv.audit = audit.Store{DB: dead}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, truncateJSON("project_a", ownerPassword)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"truncated"`) {
		t.Fatalf("audit failure returned truncated: %s", rec.Body.String())
	}
	if cat.TruncateCalls != 1 {
		t.Fatalf("SQL must complete before audit fail: %d", cat.TruncateCalls)
	}
}

func TestPostgresTruncateWrongMethodsAre405(t *testing.T) {
	cat := truncateCatalog()
	srv := truncateServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(method, postgresTruncatePath, cookie, csrf, truncateJSON("project_a", ownerPassword)))
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
	if cat.TruncateCalls != 0 {
		t.Fatalf("wrong methods must not truncate: %d", cat.TruncateCalls)
	}
}
