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
	"time"

	"github.com/SSujitX/redgres/internal/audit"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/SSujitX/redgres/internal/secrets"
)

const postgresDuplicatePath = "/api/v1/postgres/databases/project_a/duplicate"
const postgresDuplicateBody = `{"database":"project_a_copy","owner":"app_project_a_copy"}`
const duplicateCanaryPassword = "canary-duplicate-password-secret"

func duplicateRow() postgresadmin.CatalogRow {
	return postgresadmin.CatalogRow{Name: "project_a", Owner: "app_project_a", AllowConn: true, OwnerCanLogin: true}
}

func duplicateMemory(t *testing.T) *postgresadmin.MemoryCatalog {
	t.Helper()
	return &postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{duplicateRow()}}
}

func duplicateServer(t *testing.T, cat *postgresadmin.MemoryCatalog, vaultKey string) *Server {
	t.Helper()
	return createServer(t, cat, vaultKey)
}

func TestPostgresDuplicateRequiresSession(t *testing.T) {
	fx := loadPython49(t)
	srv := duplicateServer(t, duplicateMemory(t), secrets.DeriveVaultKey(fx.SessionSecret))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, postgresDuplicatePath, strings.NewReader(postgresDuplicateBody)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("401 leaked credential: %s", rec.Body.String())
	}
}

func TestPostgresDuplicateRequiresCSRF(t *testing.T) {
	fx := loadPython49(t)
	srv := duplicateServer(t, duplicateMemory(t), secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, "", postgresDuplicateBody))
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

func TestPostgresDuplicateCapabilityIsProvision(t *testing.T) {
	if !hasCapability("postgres.provision") {
		t.Fatal("postgres.provision must be granted")
	}
	srv, _ := testServer(t, nil)
	reached := false
	handler := srv.requireCapability("postgres.export")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, postgresDuplicatePath, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
	if reached {
		t.Fatal("handler ran without the capability")
	}
}

func TestPostgresDuplicateUnknownFieldIs400(t *testing.T) {
	fx := loadPython49(t)
	cat := duplicateMemory(t)
	srv := duplicateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, raw := range []string{
		`{"database":"project_a_copy","owner":"app_project_a_copy","new_owner_password":"nope"}`,
		`{"database":"project_a_copy","owner":"app_project_a_copy","create_owner_role":true}`,
		`{"database":"project_a_copy","owner":"app_project_a_copy","password":"` + duplicateCanaryPassword + `"}`,
		`{"database":"project_a_copy","owner":"app_project_a_copy","confirmation":"project_a"}`,
		`{"database":"project_a_copy","owner":"app_project_a_copy","owner_password":"nope"}`,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, raw))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Unknown field") {
			t.Fatalf("body = %s", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), duplicateCanaryPassword) {
			t.Fatalf("error JSON leaked canary: %s", rec.Body.String())
		}
	}
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 {
		t.Fatal("unknown field must not DDL")
	}
	var n int
	if err := srv.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action = 'postgres.database.duplicate'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("unknown field must not audit: %d", n)
	}
}

func TestPostgresDuplicateSameNameIs400(t *testing.T) {
	fx := loadPython49(t)
	cat := duplicateMemory(t)
	srv := duplicateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, `{"database":"project_a","owner":"app_project_a_copy"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeValidationError || body.Error.Message != postgresDuplicateSameNameMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if body.Error.Fields["database"] != "invalid" {
		t.Fatalf("fields = %#v", body.Error.Fields)
	}
	if cat.CreateDatabaseTemplateCalls != 0 {
		t.Fatal("same name must not DDL")
	}
}

func TestPostgresDuplicateUniqueOwnerIs400(t *testing.T) {
	fx := loadPython49(t)
	cat := duplicateMemory(t)
	srv := duplicateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, `{"database":"project_a_copy","owner":"app_project_a"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeValidationError || body.Error.Message != "Choose a different project user than the source database owner." {
		t.Fatalf("error = %#v", body.Error)
	}
	if body.Error.Fields["owner"] != "invalid" {
		t.Fatalf("fields = %#v", body.Error.Fields)
	}
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 {
		t.Fatal("unique owner must not DDL")
	}
}

func TestPostgresDuplicateProtectedNewNameIs403NoDDL(t *testing.T) {
	fx := loadPython49(t)
	cat := duplicateMemory(t)
	srv := duplicateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, `{"database":"postgres","owner":"app_project_a_copy"}`))
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
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 {
		t.Fatal("protected new name must not DDL")
	}
}

func TestPostgresDuplicateProtectedSourceIs404NoDDL(t *testing.T) {
	fx := loadPython49(t)
	cat := &postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "postgres", Owner: "app_project_a", AllowConn: true, OwnerCanLogin: true},
	}}
	srv := duplicateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/postgres/databases/postgres/duplicate", cookie, csrf, postgresDuplicateBody))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if cat.CreateDatabaseTemplateCalls != 0 {
		t.Fatal("protected source must not DDL")
	}
}

func TestPostgresDuplicate201NoStoreOneTimeFalse(t *testing.T) {
	fx := loadPython49(t)
	cat := duplicateMemory(t)
	srv := duplicateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, postgresDuplicateBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	if !strings.Contains(rec.Body.String(), `"one_time":false`) {
		t.Fatalf("one_time must be JSON false: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"warning"`) || strings.Contains(rec.Body.String(), `"transferred_from"`) {
		t.Fatal("must not copy sibling warning/transferred_from")
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	resource, _ := body["resource"].(map[string]any)
	if resource["type"] != "postgres_database" || resource["name"] != "project_a_copy" {
		t.Fatalf("resource = %#v", resource)
	}
	cred, _ := body["credential"].(map[string]any)
	password, _ := cred["password"].(string)
	if cred["username"] != "app_project_a_copy" || password == "" || cred["one_time"] != false {
		t.Fatalf("credential = %#v", cred)
	}
	urls, _ := cred["urls"].(map[string]any)
	if urls["direct"] != "postgresql://app_project_a_copy:"+password+"@db.example.com:5432/project_a_copy?sslmode=require" {
		t.Fatalf("direct = %#v", urls["direct"])
	}
	if urls["pooled"] != "postgresql://app_project_a_copy:"+password+"@db.example.com:6432/project_a_copy?sslmode=require" {
		t.Fatalf("pooled = %#v", urls["pooled"])
	}
	id, _ := body["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'postgres.database.duplicate' AND outcome = 'success' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "project_a_copy" || meta["owner"] != "app_project_a_copy" || meta["source"] != "project_a" {
		t.Fatalf("metadata = %s", metadata)
	}
	if len(meta) != 3 {
		t.Fatalf("metadata keys = %#v", meta)
	}
	if strings.Contains(metadata, password) || strings.Contains(metadata, "postgresql://") || strings.Contains(metadata, duplicateCanaryPassword) {
		t.Fatalf("audit leaked secret: %s", metadata)
	}
}

func TestPostgresDuplicateIsolationMismatchIs503(t *testing.T) {
	fx := loadPython49(t)
	cat := &postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{duplicateRow()},
		SnapshotSeq: []postgresadmin.OwnershipSnapshot{
			{Owner: "app_project_a", Datacl: ""},
			{Owner: "other_role", Datacl: "{changed}"},
		},
	}
	srv := duplicateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, postgresDuplicateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeDependencyUnavailable || body.Error.Message != postgresDuplicateIsolationMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if strings.Contains(rec.Body.String(), `"credential"`) {
		t.Fatalf("503 leaked credential: %s", rec.Body.String())
	}
	if len(cat.DroppedDatabases) != 1 || cat.DroppedDatabases[0] != "project_a_copy" {
		t.Fatalf("dropped = %#v", cat.DroppedDatabases)
	}
}

func TestPostgresDuplicateInProgressCopy(t *testing.T) {
	fx := loadPython49(t)
	cat := &postgresadmin.MemoryCatalog{
		Rows:            []postgresadmin.CatalogRow{duplicateRow()},
		TemplateStarted: make(chan struct{}, 1),
		TemplateHold:    make(chan struct{}),
	}
	srv := duplicateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, postgresDuplicateBody))
		done <- rec
	}()
	select {
	case <-cat.TemplateStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first duplicate did not reach TEMPLATE")
	}
	second := httptest.NewRecorder()
	h.ServeHTTP(second, authed(http.MethodPost, "/api/v1/postgres/databases/project_a/duplicate", cookie, csrf, `{"database":"project_b_copy","owner":"app_project_b_copy"}`))
	if second.Code != http.StatusConflict {
		t.Fatalf("second status = %d %s", second.Code, second.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(second.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeOperationInProgress || body.Error.Message != postgresDuplicateInProgressMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if body.Error.Message == postgresRotateInProgressMessage {
		t.Fatal("must not reuse rotate 409 copy")
	}
	close(cat.TemplateHold)
	select {
	case rec := <-done:
		if rec.Code != http.StatusCreated {
			t.Fatalf("first status = %d %s", rec.Code, rec.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first duplicate blocked")
	}
}

func TestPostgresRotate409CopyUnchanged(t *testing.T) {
	fx := loadPython49(t)
	cat := &postgresadmin.MemoryCatalog{
		Rows:         []postgresadmin.CatalogRow{duplicateRow()},
		AlterStarted: make(chan struct{}, 1),
		AlterHold:    make(chan struct{}),
	}
	srv := rotateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPost, postgresRotatePath, cookie, csrf, postgresRotateBody))
		done <- rec
	}()
	select {
	case <-cat.AlterStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first rotate did not reach ALTER")
	}
	second := httptest.NewRecorder()
	h.ServeHTTP(second, authed(http.MethodPost, postgresRotatePath, cookie, csrf, postgresRotateBody))
	if second.Code != http.StatusConflict {
		t.Fatalf("second status = %d %s", second.Code, second.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(second.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeOperationInProgress || body.Error.Message != postgresRotateInProgressMessage {
		t.Fatalf("rotate 409 copy changed: %#v", body.Error)
	}
	if body.Error.Message == postgresDuplicateInProgressMessage {
		t.Fatal("rotate must keep its own 409 copy")
	}
	close(cat.AlterHold)
	select {
	case rec := <-done:
		if rec.Code != http.StatusOK {
			t.Fatalf("first status = %d %s", rec.Code, rec.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first rotate blocked")
	}
}

func TestPostgresDuplicateVaultInsertFailureCompensates(t *testing.T) {
	fx := loadPython49(t)
	canary := "postgresql://canary-token:secret@10.0.0.1/db"
	cat := &postgresadmin.MemoryCatalog{
		Rows:                []postgresadmin.CatalogRow{duplicateRow()},
		InsertCredentialErr: errors.New(canary),
	}
	srv := duplicateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, postgresDuplicateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "canary-token") || strings.Contains(rec.Body.String(), `"credential"`) {
		t.Fatalf("leaked canary: %s", rec.Body.String())
	}
	if cat.DropDatabaseCalls != 1 || cat.DroppedDatabases[0] != "project_a_copy" {
		t.Fatalf("dropped = %#v", cat.DroppedDatabases)
	}
	if cat.DropRoleCalls != 1 || cat.DroppedRoles[0] != "app_project_a_copy" {
		t.Fatalf("roles = %#v", cat.DroppedRoles)
	}
}

func TestPostgresDuplicateMissingVaultKeyIs503NoDDL(t *testing.T) {
	cat := duplicateMemory(t)
	srv := duplicateServer(t, cat, "")
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, postgresDuplicateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 {
		t.Fatal("missing vault key must not DDL")
	}
}

func TestPostgresDuplicateWrongMethodsAre405(t *testing.T) {
	fx := loadPython49(t)
	cat := duplicateMemory(t)
	srv := duplicateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(method, postgresDuplicatePath, cookie, csrf, postgresDuplicateBody))
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
	h.ServeHTTP(post, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, postgresDuplicateBody))
	if post.Code != http.StatusCreated {
		t.Fatalf("POST status = %d %s", post.Code, post.Body.String())
	}
	item := httptest.NewRecorder()
	h.ServeHTTP(item, authed(http.MethodPost, "/api/v1/postgres/databases/project_a", cookie, csrf, postgresDuplicateBody))
	if item.Code != http.StatusMethodNotAllowed {
		t.Fatalf("item POST status = %d %s", item.Code, item.Body.String())
	}
}

func TestPostgresDuplicateNilAdapterAuditsFailure(t *testing.T) {
	srv, _ := testServer(t, nil)
	srv.cfg.PostgresDatabase = "postgres"
	srv.cfg.PostgresUser = "redgres_console"
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, postgresDuplicateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) {
		t.Fatalf("503 leaked credential: %s", rec.Body.String())
	}
	var outcome, metadata string
	if err := srv.db.QueryRow(`SELECT outcome, metadata FROM audit_events WHERE action = 'postgres.database.duplicate' ORDER BY id DESC LIMIT 1`).Scan(&outcome, &metadata); err != nil {
		t.Fatal(err)
	}
	if outcome != "failure" {
		t.Fatalf("outcome = %s", outcome)
	}
	if strings.Contains(metadata, duplicateCanaryPassword) {
		t.Fatalf("audit leaked canary: %s", metadata)
	}
}

func TestPostgresDuplicateAuditFailClosed(t *testing.T) {
	fx := loadPython49(t)
	cat := duplicateMemory(t)
	srv := duplicateServer(t, cat, secrets.DeriveVaultKey(fx.SessionSecret))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	dead, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dead-audit-duplicate.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dead.Close() })
	_ = dead.Close()
	srv.audit = audit.Store{DB: dead}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, postgresDuplicatePath, cookie, csrf, postgresDuplicateBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("audit failure returned credential: %s", rec.Body.String())
	}
	if cat.InsertCalls != 1 {
		t.Fatalf("cluster+vault must remain: inserts=%d", cat.InsertCalls)
	}
	if cat.DropDatabaseCalls != 0 || cat.DropRoleCalls != 0 {
		t.Fatal("audit-fail must not compensate")
	}
}
