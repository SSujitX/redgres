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
	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
)

const (
	postgresDropPath   = "/api/v1/postgres/databases/project_a"
	postgresDropCanary = "wrong-canary-password"
)

func dropCatalog() *postgresadmin.MemoryCatalog {
	return &postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{
			{Name: "project_a", Owner: "app_project_a", AllowConn: true},
			{Name: "postgres", Owner: "postgres", AllowConn: true},
		},
		OwnedCount: 0,
	}
}

func dropService(cat *postgresadmin.MemoryCatalog) *postgresadmin.Service {
	return postgresadmin.NewService(cat, postgresadmin.NewPolicy(config.Config{PostgresDatabase: "postgres"}))
}

func dropServer(t *testing.T, cat *postgresadmin.MemoryCatalog, enabled bool) *Server {
	t.Helper()
	srv := testServerWithPostgres(t, dropService(cat))
	srv.cfg.FeaturePostgresDrop = enabled
	srv.cfg.FeaturePostgresTruncate = true
	return srv
}

func dropJSON(database, password string) string {
	return `{"database_confirmation":"` + database + `","owner_password":"` + password + `"}`
}

func TestPostgresDropFlagOffBeforeDecode(t *testing.T) {
	cat := dropCatalog()
	srv := dropServer(t, cat, false)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, `{"database_confirmation":"project_a","owner_password":"`+ownerPassword+`","backup_confirmed":true`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeForbidden || body.Error.Message != postgresDropOffMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	if cat.DropCalls != 0 || cat.TerminateCalls != 0 || cat.DropDatabaseCalls != 0 {
		t.Fatalf("PostgreSQL on flag off: drop=%d terminate=%d compensate=%d", cat.DropCalls, cat.TerminateCalls, cat.DropDatabaseCalls)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("flag off wrote audit: %d -> %d", before, after)
	}
}

func TestPostgresDropRequiresCSRF(t *testing.T) {
	cat := dropCatalog()
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, "", dropJSON("project_a", ownerPassword)))
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
	if cat.DropCalls != 0 || cat.TerminateCalls != 0 {
		t.Fatalf("SQL before CSRF: drop=%d terminate=%d", cat.DropCalls, cat.TerminateCalls)
	}
}

func TestPostgresDropUnknownFields(t *testing.T) {
	cat := dropCatalog()
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	for _, extra := range []string{
		`"confirmation":"project_a"`,
		`"backup_confirmed":true`,
		`"drop_role":true`,
		`"force":true`,
		`"cascade":true`,
		`"password":"x"`,
	} {
		rec := httptest.NewRecorder()
		body := `{"database_confirmation":"project_a","owner_password":"` + ownerPassword + `",` + extra + `}`
		h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, body))
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
	if cat.DropCalls != 0 || cat.TerminateCalls != 0 {
		t.Fatalf("SQL on unknown field: drop=%d terminate=%d", cat.DropCalls, cat.TerminateCalls)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("unknown field wrote audit: %d -> %d", before, after)
	}
}

func TestPostgresDropConfirmationMismatchNoAudit(t *testing.T) {
	cat := dropCatalog()
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	for _, body := range []string{
		dropJSON("Project_a", ownerPassword),
		dropJSON("", ownerPassword),
		dropJSON("project_b", ownerPassword),
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
		}
		var errBody errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
			t.Fatal(err)
		}
		if errBody.Error.Code != CodeValidationError || errBody.Error.Message != postgresDropConfirmMessage {
			t.Fatalf("error = %#v", errBody.Error)
		}
		if errBody.Error.Fields["database_confirmation"] != "invalid" {
			t.Fatalf("fields = %#v", errBody.Error.Fields)
		}
	}
	if cat.DropCalls != 0 || cat.TerminateCalls != 0 {
		t.Fatalf("PostgreSQL on confirmation fail: drop=%d terminate=%d", cat.DropCalls, cat.TerminateCalls)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("confirmation wrote audit: %d -> %d", before, after)
	}
}

func TestPostgresDropReauthRequiredMetadataDatabaseOnly(t *testing.T) {
	cat := dropCatalog()
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	attemptsBefore := countLoginAttempts(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, dropJSON("project_a", postgresDropCanary)))
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
	if strings.Contains(raw, postgresDropCanary) || strings.Contains(raw, ownerPassword) {
		t.Fatalf("403 leaked password: %s", raw)
	}
	if rec.Code == http.StatusTooManyRequests {
		t.Fatal("reauth must not return 429")
	}
	if cat.DropCalls != 0 || cat.TerminateCalls != 0 || cat.DropDatabaseCalls != 0 {
		t.Fatalf("SQL before successful reauth: drop=%d terminate=%d compensate=%d", cat.DropCalls, cat.TerminateCalls, cat.DropDatabaseCalls)
	}
	var metadata, action, outcome, target string
	if err := srv.db.QueryRow(`SELECT action, target, outcome, metadata FROM audit_events WHERE action = 'postgres.database.drop' ORDER BY id DESC LIMIT 1`).Scan(&action, &target, &outcome, &metadata); err != nil {
		t.Fatal(err)
	}
	if action != "postgres.database.drop" || outcome != "failure" || target != "project_a" {
		t.Fatalf("audit = %s/%s/%s", action, target, outcome)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "project_a" || len(meta) != 1 {
		t.Fatalf("metadata = %s", metadata)
	}
	if strings.Contains(metadata, postgresDropCanary) || strings.Contains(metadata, ownerPassword) || strings.Contains(metadata, "password") {
		t.Fatalf("audit leaked secret: %s", metadata)
	}
	if after := countLoginAttempts(t, srv); after != attemptsBefore {
		t.Fatalf("login_attempts changed %d -> %d", attemptsBefore, after)
	}
}

func TestPostgresDropProtected404NoSQL(t *testing.T) {
	cat := dropCatalog()
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/postgres/databases/postgres", cookie, csrf, dropJSON("postgres", ownerPassword)))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "protected_resource") {
		t.Fatal("protected drop must be 404 not protected_resource")
	}
	if cat.TerminateCalls != 0 || cat.DropCalls != 0 || cat.DropDatabaseCalls != 0 {
		t.Fatalf("SQL on protected: terminate=%d drop=%d compensate=%d", cat.TerminateCalls, cat.DropCalls, cat.DropDatabaseCalls)
	}
}

func TestPostgresDropMissing404(t *testing.T) {
	cat := dropCatalog()
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/postgres/databases/missing_db", cookie, csrf, dropJSON("missing_db", ownerPassword)))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if cat.TerminateCalls != 0 || cat.DropCalls != 0 {
		t.Fatalf("SQL on missing: terminate=%d drop=%d", cat.TerminateCalls, cat.DropCalls)
	}
}

func TestPostgresDropOwnedCountNonZeroOmitsDroppedRole(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 2
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, dropJSON("project_a", ownerPassword)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["dropped"] != "project_a" {
		t.Fatalf("body = %#v", body)
	}
	if _, present := body["dropped_role"]; present {
		t.Fatalf("dropped_role must be omitted: %#v", body)
	}
	if _, present := body["message"]; present {
		t.Fatalf("must not include sibling message: %#v", body)
	}
	if cat.DropRoleCalls != 0 || cat.DeleteCredentialCalls != 0 {
		t.Fatalf("role=%d vault=%d", cat.DropRoleCalls, cat.DeleteCredentialCalls)
	}
	if cat.DropCalls != 1 || cat.DropDatabaseCalls != 0 {
		t.Fatalf("operator drop=%d compensate=%d", cat.DropCalls, cat.DropDatabaseCalls)
	}
	if cat.LastDropSQL != `DROP DATABASE "project_a"` || strings.Contains(cat.LastDropSQL, "IF EXISTS") || strings.Contains(strings.ToUpper(cat.LastDropSQL), "FORCE") {
		t.Fatalf("sql = %s", cat.LastDropSQL)
	}
}

func TestPostgresDropOwnerDeniedOmitsDroppedRole(t *testing.T) {
	cat := &postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{
			{Name: "project_a", Owner: "postgres", AllowConn: true},
		},
		OwnedCount: 0,
	}
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, dropJSON("project_a", ownerPassword)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("denied owner must be 404 like GET details: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "protected_resource") {
		t.Fatal("must be 404 not protected_resource")
	}
	if cat.TerminateCalls != 0 || cat.DropCalls != 0 || cat.DropRoleCalls != 0 || cat.DeleteCredentialCalls != 0 {
		t.Fatalf("SQL on denied owner: terminate=%d drop=%d role=%d vault=%d", cat.TerminateCalls, cat.DropCalls, cat.DropRoleCalls, cat.DeleteCredentialCalls)
	}
}

func TestPostgresDropCountZeroIncludesDroppedRole(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 0
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, dropJSON("project_a", ownerPassword)))
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
	if body["dropped"] != "project_a" || body["dropped_role"] != "app_project_a" {
		t.Fatalf("body = %#v", body)
	}
	id, _ := body["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
	if _, present := body["message"]; present {
		t.Fatalf("must not include sibling message: %#v", body)
	}
	if strings.Contains(rec.Body.String(), ownerPassword) {
		t.Fatalf("200 leaked password: %s", rec.Body.String())
	}
	if cat.TerminateCalls != 1 || cat.DropCalls != 1 || cat.DropDatabaseCalls != 0 {
		t.Fatalf("terminate=%d drop=%d compensate=%d", cat.TerminateCalls, cat.DropCalls, cat.DropDatabaseCalls)
	}
	if cat.DropRoleCalls != 1 || cat.DeleteCredentialCalls != 1 {
		t.Fatalf("role=%d vault=%d", cat.DropRoleCalls, cat.DeleteCredentialCalls)
	}
	if after := countAuditEvents(t, srv); after != before+1 {
		t.Fatalf("audit count %d -> %d", before, after)
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'postgres.database.drop' AND outcome = 'success' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "project_a" || meta["owner"] != "app_project_a" || meta["dropped_role"] != "app_project_a" {
		t.Fatalf("success metadata = %s", metadata)
	}
	if len(meta) != 3 {
		t.Fatalf("metadata keys = %#v", meta)
	}
}

func TestPostgresDropRoleFailure503(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 0
	cat.DropRoleErr = errors.New("postgresql://canary:secret@127.0.0.1/db")
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, dropJSON("project_a", ownerPassword)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeDependencyUnavailable || body.Error.Message != postgresDropRoleFailedMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "canary") || strings.Contains(raw, "secret") || strings.Contains(raw, ownerPassword) {
		t.Fatalf("leaked: %s", raw)
	}
	if cat.DropCalls != 1 || cat.DropRoleCalls != 1 || cat.DeleteCredentialCalls != 0 {
		t.Fatalf("drop=%d role=%d vault=%d", cat.DropCalls, cat.DropRoleCalls, cat.DeleteCredentialCalls)
	}
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'postgres.database.drop' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "project_a" || len(meta) != 1 {
		t.Fatalf("failure metadata = %s", metadata)
	}
}

func TestPostgresDropVaultFailure503(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 0
	cat.DeleteCredentialErr = errors.New("postgresql://canary:secret@127.0.0.1/db")
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, dropJSON("project_a", ownerPassword)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeDependencyUnavailable || body.Error.Message != postgresDropVaultFailedMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "canary") || strings.Contains(raw, "secret") {
		t.Fatalf("leaked: %s", raw)
	}
	if cat.DropCalls != 1 || cat.DropRoleCalls != 1 || cat.DeleteCredentialCalls != 1 {
		t.Fatalf("drop=%d role=%d vault=%d", cat.DropCalls, cat.DropRoleCalls, cat.DeleteCredentialCalls)
	}
}

func TestPostgresDropInProgress409(t *testing.T) {
	cat := &postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{
			{Name: "project_a", Owner: "app_project_a", AllowConn: true},
		},
		OwnedCount:  1,
		DropStarted: make(chan struct{}, 1),
		DropHold:    make(chan struct{}),
	}
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, dropJSON("project_a", ownerPassword)))
		done <- rec
	}()
	select {
	case <-cat.DropStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first drop did not reach SQL")
	}
	second := httptest.NewRecorder()
	h.ServeHTTP(second, authed(http.MethodDelete, postgresDropPath, cookie, csrf, dropJSON("project_a", ownerPassword)))
	if second.Code != http.StatusConflict {
		t.Fatalf("second status = %d %s", second.Code, second.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(second.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeOperationInProgress || body.Error.Message != postgresDropInProgressMessage {
		t.Fatalf("error = %#v", body.Error)
	}
	if body.Error.Message == postgresTruncateInProgressMessage || body.Error.Message == postgresRotateInProgressMessage || body.Error.Message == postgresDuplicateInProgressMessage {
		t.Fatal("must not reuse truncate/rotate/duplicate 409 copy")
	}
	close(cat.DropHold)
	select {
	case rec := <-done:
		if rec.Code != http.StatusOK {
			t.Fatalf("first status = %d %s", rec.Code, rec.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first drop blocked")
	}
}

func TestPostgresDropTruncateLockCollisionBothDirections(t *testing.T) {
	cat := &postgresadmin.MemoryCatalog{
		Rows: []postgresadmin.CatalogRow{
			{Name: "project_a", Owner: "app_project_a", AllowConn: true},
		},
		Tables: map[string][]postgresadmin.TableItem{
			"project_a": {{Schema: "public", Name: "items"}},
		},
		OwnedCount:      1,
		TruncateStarted: make(chan struct{}, 1),
		TruncateHold:    make(chan struct{}),
		DropStarted:     make(chan struct{}, 1),
		DropHold:        make(chan struct{}),
	}
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)

	truncDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, truncateJSON("project_a", ownerPassword)))
		truncDone <- rec
	}()
	select {
	case <-cat.TruncateStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("truncate did not reach SQL")
	}
	dropWhileTrunc := httptest.NewRecorder()
	h.ServeHTTP(dropWhileTrunc, authed(http.MethodDelete, postgresDropPath, cookie, csrf, dropJSON("project_a", ownerPassword)))
	if dropWhileTrunc.Code != http.StatusConflict {
		t.Fatalf("drop-during-truncate status = %d %s", dropWhileTrunc.Code, dropWhileTrunc.Body.String())
	}
	var truncBody errorBody
	if err := json.Unmarshal(dropWhileTrunc.Body.Bytes(), &truncBody); err != nil {
		t.Fatal(err)
	}
	if truncBody.Error.Message != postgresTruncateInProgressMessage {
		t.Fatalf("drop-during-truncate error = %#v", truncBody.Error)
	}
	close(cat.TruncateHold)
	select {
	case rec := <-truncDone:
		if rec.Code != http.StatusOK {
			t.Fatalf("truncate status = %d %s", rec.Code, rec.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("truncate blocked")
	}

	dropDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, dropJSON("project_a", ownerPassword)))
		dropDone <- rec
	}()
	select {
	case <-cat.DropStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("drop did not reach SQL")
	}
	truncWhileDrop := httptest.NewRecorder()
	h.ServeHTTP(truncWhileDrop, authed(http.MethodPost, postgresTruncatePath, cookie, csrf, truncateJSON("project_a", ownerPassword)))
	if truncWhileDrop.Code != http.StatusConflict {
		t.Fatalf("truncate-during-drop status = %d %s", truncWhileDrop.Code, truncWhileDrop.Body.String())
	}
	var dropBody errorBody
	if err := json.Unmarshal(truncWhileDrop.Body.Bytes(), &dropBody); err != nil {
		t.Fatal(err)
	}
	if dropBody.Error.Message != postgresDropInProgressMessage {
		t.Fatalf("truncate-during-drop error = %#v", dropBody.Error)
	}
	close(cat.DropHold)
	select {
	case rec := <-dropDone:
		if rec.Code != http.StatusOK {
			t.Fatalf("drop status = %d %s", rec.Code, rec.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("drop blocked")
	}
}

func TestPostgresDropAuditFailClosed(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 0
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	dead, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dead-audit-drop.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dead.Close() })
	_ = dead.Close()
	srv.audit = audit.Store{DB: dead}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, postgresDropPath, cookie, csrf, dropJSON("project_a", ownerPassword)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"dropped"`) {
		t.Fatalf("audit failure returned dropped: %s", rec.Body.String())
	}
	if cat.DropCalls != 1 {
		t.Fatalf("SQL must complete before audit fail: %d", cat.DropCalls)
	}
}

func TestPostgresDropMethodsAndGetStill200(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 1
	srv := dropServer(t, cat, true)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	item := "/api/v1/postgres/databases/project_a"
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(method, item, cookie, csrf, dropJSON("project_a", ownerPassword)))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s {db} status = %d %s", method, rec.Code, rec.Body.String())
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != CodeMethodNotAllowed {
			t.Fatalf("%s code = %q", method, body.Error.Code)
		}
	}
	collection := httptest.NewRecorder()
	h.ServeHTTP(collection, authed(http.MethodDelete, "/api/v1/postgres/databases", cookie, csrf, dropJSON("project_a", ownerPassword)))
	if collection.Code != http.StatusMethodNotAllowed {
		t.Fatalf("collection DELETE status = %d %s", collection.Code, collection.Body.String())
	}
	get := httptest.NewRecorder()
	h.ServeHTTP(get, authed(http.MethodGet, item, cookie, csrf, ""))
	if get.Code != http.StatusOK {
		t.Fatalf("GET {db} status = %d %s", get.Code, get.Body.String())
	}
	if cat.DropCalls != 0 {
		t.Fatalf("wrong methods must not drop: %d", cat.DropCalls)
	}
	del := httptest.NewRecorder()
	h.ServeHTTP(del, authed(http.MethodDelete, item, cookie, csrf, dropJSON("project_a", ownerPassword)))
	if del.Code != http.StatusOK {
		t.Fatalf("DELETE {db} status = %d %s", del.Code, del.Body.String())
	}
	if cat.DropCalls != 1 {
		t.Fatalf("DELETE must drop: %d", cat.DropCalls)
	}
}
