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
	"github.com/SSujitX/redgres/internal/redisadmin"
)

func TestRedisUserPatchNamedEmptyCommandsArray(t *testing.T) {
	mem := &redisadmin.MemoryClient{ACLLines: []string{enableUserACLLine}}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPatch, redisPatchPath, cookie, csrf, `{"key_pattern":"project_b","preset":"read-only","commands":[]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	assertPatchSuccessBody(t, rec, "project_b:*", "read-only", "")
	if len(mem.ACLSetUserCalls) != 1 {
		t.Fatalf("ACLSetUser calls = %d", len(mem.ACLSetUserCalls))
	}
	assertHTTPUpdateRulesSafe(t, mem.ACLSetUserCalls[0].Rules)
}

func TestRedisUserPatchCustomAuditFailClosed(t *testing.T) {
	mem := &redisadmin.MemoryClient{ACLLines: []string{enableUserACLLine}}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	dead, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dead-audit-patch-custom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dead.Close() })
	_ = dead.Close()
	srv.audit = audit.Store{DB: dead}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPatch, redisPatchPath, cookie, csrf, `{"key_pattern":"project_b","preset":"custom","commands":["echo","get","ping"]}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["user"]; ok {
		t.Fatalf("audit failure returned user: %s", rec.Body.Bytes())
	}
	if len(mem.ACLSetUserCalls) != 1 {
		t.Fatalf("expected SETUSER before audit fail, calls = %d", len(mem.ACLSetUserCalls))
	}
	assertHTTPUpdateRulesSafe(t, mem.ACLSetUserCalls[0].Rules)
}

func TestRedisCommandsRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/redis/commands", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"commands", "presets", "state", "users", "user", "reason", "credential"} {
		if _, ok := raw[key]; ok {
			t.Fatalf("401 leaked %s: %s", key, rec.Body.Bytes())
		}
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeUnauthorized {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestRedisCommandsMethodNotAllowed(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(method, "/api/v1/redis/commands", cookie, csrf, "{}"))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d body = %s", method, rec.Code, rec.Body.Bytes())
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != CodeMethodNotAllowed {
			t.Fatalf("%s code = %q", method, body.Error.Code)
		}
	}
}

func TestRedisCommandsCatalogEqualsAllowedCommands(t *testing.T) {
	mem := &redisadmin.MemoryClient{
		PingErr:       errors.New("ping should not run"),
		InfoErr:       errors.New("info should not run"),
		DBSizeErr:     errors.New("dbsize should not run"),
		ACLListErr:    errors.New("acl list should not run"),
		ACLSetUserErr: errors.New("setuser should not run"),
	}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/commands", cookie, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("GET commands wrote audit: %d -> %d", before, after)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("GET commands used SETUSER: %#v", mem.ACLSetUserCalls)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"state", "reason", "preset", "presets"} {
		if _, ok := raw[key]; ok {
			t.Fatalf("commands leaked %s: %s", key, rec.Body.Bytes())
		}
	}
	gotCmds := jsonStringSlice(raw["commands"])
	if gotCmds == nil {
		t.Fatalf("commands missing or null: %s", rec.Body.Bytes())
	}
	want := redisadmin.AllowedCommands()
	if !stringSlicesEqual(gotCmds, want) {
		t.Fatalf("commands mismatch catalog=%#v want=%#v", gotCmds, want)
	}
	id, _ := raw["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
}

func TestRedisCommandsNilAdapterOK(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/commands", cookie, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	cmds, ok := raw["commands"].([]any)
	if !ok || len(cmds) == 0 {
		t.Fatalf("commands = %#v", raw["commands"])
	}
	if _, ok := raw["state"]; ok {
		t.Fatalf("nil adapter leaked state: %s", rec.Body.Bytes())
	}
}

func TestRedisUserPatchCustom200(t *testing.T) {
	mem := &redisadmin.MemoryClient{ACLLines: []string{enableUserACLLine}}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	body := `{"key_pattern":"project_b","preset":"custom","commands":["echo","get","ping"]}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPatch, redisPatchPath, cookie, csrf, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	assertPatchSuccessBody(t, rec, "project_b:*", "custom", "")
	if len(mem.ACLSetUserCalls) != 1 {
		t.Fatalf("ACLSetUser calls = %d", len(mem.ACLSetUserCalls))
	}
	assertHTTPUpdateRulesSafe(t, mem.ACLSetUserCalls[0].Rules)
	gotCmds := httpGrantedCommands(mem.ACLSetUserCalls[0].Rules)
	if !stringSlicesEqual(gotCmds, []string{"echo", "get", "ping"}) {
		t.Fatalf("SETUSER grants = %#v", gotCmds)
	}
	assertAuditUpdateSafe(t, srv, "success", "project_a", "custom", "project_b:*", "")
	assertAuditUpdateOmitsCommands(t, srv)
}

func TestRedisUserPatchCustomRejectsDangerousAndEmpty(t *testing.T) {
	mem := &redisadmin.MemoryClient{
		ACLLines:   []string{enableUserACLLine},
		ACLListErr: errors.New("acl list should not run for invalid commands"),
	}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	cases := []struct {
		name string
		body string
	}{
		{name: "flushall", body: `{"key_pattern":"project_a","preset":"custom","commands":["flushall"]}`},
		{name: "at-all", body: `{"key_pattern":"project_a","preset":"custom","commands":["@all"]}`},
		{name: "acl", body: `{"key_pattern":"project_a","preset":"custom","commands":["acl"]}`},
		{name: "empty", body: `{"key_pattern":"project_a","preset":"custom"}`},
		{name: "empty-array", body: `{"key_pattern":"project_a","preset":"custom","commands":[]}`},
		{name: "named-plus-commands", body: `{"key_pattern":"project_a","preset":"read-only","commands":["get"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, authed(http.MethodPatch, redisPatchPath, cookie, csrf, tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
			}
			var body errorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != CodeValidationError || body.Error.Fields["commands"] == "" {
				t.Fatalf("error = %#v", body.Error)
			}
		})
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("SETUSER on command reject: %#v", mem.ACLSetUserCalls)
	}
}

func TestRedisUsersCreateCustom201(t *testing.T) {
	mem := &redisadmin.MemoryClient{}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	body := `{"username":"project_a","key_pattern":"project_a","preset":"custom","commands":["echo","get","ping"]}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/redis/users", cookie, csrf, body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store, max-age=0" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("Pragma = %q", rec.Header().Get("Pragma"))
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["state"]; ok {
		t.Fatalf("create leaked state: %s", rec.Body.Bytes())
	}
	user, _ := raw["user"].(map[string]any)
	if user["username"] != "project_a" || user["enabled"] != true || user["key_pattern"] != "project_a:*" {
		t.Fatalf("user = %#v", user)
	}
	if user["preset"] != "custom" || user["protected"] != false || user["rule_fidelity"] != "exact" {
		t.Fatalf("user labels = %#v", user)
	}
	if _, ok := user["commands"]; ok {
		t.Fatalf("create summary leaked commands: %#v", user)
	}
	if _, ok := user["queue_kind"]; ok {
		t.Fatalf("queue_kind present for custom: %#v", user)
	}
	cred, _ := raw["credential"].(map[string]any)
	password, _ := cred["password"].(string)
	if cred["username"] != "project_a" || cred["one_time"] != true || len(password) != 32 {
		t.Fatalf("credential = %#v", cred)
	}
	if len(mem.ACLSetUserCalls) != 1 {
		t.Fatalf("ACLSetUser calls = %d", len(mem.ACLSetUserCalls))
	}
	assertHTTPCreateRulesSafe(t, mem.ACLSetUserCalls[0].Rules, password, "project_a:*", []string{"echo", "get", "ping"})
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'redis.user.create' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metadata, `"username":"project_a"`) || !strings.Contains(metadata, `"preset":"custom"`) || !strings.Contains(metadata, `"key_pattern":"project_a:*"`) {
		t.Fatalf("audit metadata = %s", metadata)
	}
	if strings.Contains(metadata, `"commands"`) || strings.Contains(metadata, `"queue_kind"`) || strings.Contains(metadata, password) || strings.Contains(metadata, ">") {
		t.Fatalf("audit leaked commands or secret: %s", metadata)
	}
}

func TestRedisUsersCreateCustomMatchingNamedSetInfersPreset(t *testing.T) {
	mem := &redisadmin.MemoryClient{}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	var cmds []string
	for _, p := range redisadmin.NamedPresets() {
		if p.Preset == "read-only" {
			cmds = p.Commands
			break
		}
	}
	if len(cmds) == 0 {
		t.Fatal("read-only catalog missing")
	}
	rawCmds, err := json.Marshal(cmds)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"username":"project_a","key_pattern":"project_a","preset":"custom","commands":` + string(rawCmds) + `}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/redis/users", cookie, csrf, body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	user, _ := raw["user"].(map[string]any)
	if user["preset"] != "read-only" {
		t.Fatalf("inferPreset = %#v want read-only", user["preset"])
	}
}

func TestRedisUsersCreateNamedCommandsRejectedBeforeRedis(t *testing.T) {
	mem := &redisadmin.MemoryClient{ACLListErr: errors.New("acl list should not run")}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/redis/users", cookie, csrf, `{"username":"project_a","key_pattern":"project_a","preset":"read-only","commands":["get"]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeValidationError || body.Error.Fields["commands"] == "" {
		t.Fatalf("error = %#v", body.Error)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("SETUSER on named+commands: %#v", mem.ACLSetUserCalls)
	}
}

func TestRedisUsersCreateCustomRejectsBeforeRedis(t *testing.T) {
	mem := &redisadmin.MemoryClient{ACLListErr: errors.New("acl list should not run")}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	cases := []struct {
		name  string
		body  string
		field string
	}{
		{name: "omitted", body: `{"username":"project_a","key_pattern":"project_a","preset":"custom"}`, field: "commands"},
		{name: "empty-array", body: `{"username":"project_a","key_pattern":"project_a","preset":"custom","commands":[]}`, field: "commands"},
		{name: "flushall", body: `{"username":"project_a","key_pattern":"project_a","preset":"custom","commands":["flushall"]}`, field: "commands"},
		{name: "at-all", body: `{"username":"project_a","key_pattern":"project_a","preset":"custom","commands":["@all"]}`, field: "commands"},
		{name: "acl", body: `{"username":"project_a","key_pattern":"project_a","preset":"custom","commands":["acl"]}`, field: "commands"},
		{name: "queue-kind", body: `{"username":"project_a","key_pattern":"project_a","preset":"custom","commands":["ping"],"queue_kind":"lists"}`, field: "queue_kind"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/redis/users", cookie, csrf, tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
			}
			var body errorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != CodeValidationError || body.Error.Fields[tc.field] == "" {
				t.Fatalf("error = %#v want fields.%s", body.Error, tc.field)
			}
		})
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("SETUSER on command reject: %#v", mem.ACLSetUserCalls)
	}
}

func TestRedisUsersCreateCustomAuditFailClosed(t *testing.T) {
	mem := &redisadmin.MemoryClient{}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	dead, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dead-audit-create-custom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dead.Close() })
	_ = dead.Close()
	srv.audit = audit.Store{DB: dead}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/redis/users", cookie, csrf, `{"username":"project_a","key_pattern":"project_a","preset":"custom","commands":["echo","get","ping"]}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if strings.Contains(rec.Body.String(), `"credential"`) || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("audit failure returned credential: %s", rec.Body.Bytes())
	}
	if len(mem.ACLSetUserCalls) != 1 {
		t.Fatalf("expected SETUSER before audit fail, calls = %d", len(mem.ACLSetUserCalls))
	}
}

func TestRedisUserPatchCustomAuthAndFailures(t *testing.T) {
	customBody := `{"key_pattern":"project_b","preset":"custom","commands":["echo","get","ping"]}`
	t.Run("csrf", func(t *testing.T) {
		srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{
			ACLLines: []string{enableUserACLLine},
		}))
		seedOwner(t, srv)
		h := srv.Handler()
		cookie, _ := login(t, h)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPatch, redisPatchPath, cookie, "", customBody))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != CodeCSRFInvalid {
			t.Fatalf("code = %q", body.Error.Code)
		}
	})
	t.Run("unauthorized", func(t *testing.T) {
		srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{
			ACLLines: []string{enableUserACLLine},
		}))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, redisPatchPath, strings.NewReader(customBody)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
		}
		var raw map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		if _, ok := raw["user"]; ok {
			t.Fatalf("401 leaked user: %s", rec.Body.Bytes())
		}
	})
	t.Run("protected", func(t *testing.T) {
		mem := &redisadmin.MemoryClient{ACLLines: []string{
			"user admin on ~* -@all +ping",
		}}
		srv := testServerWithRedis(t, redisadmin.NewServiceAdmin(mem, "ops_admin"))
		seedOwner(t, srv)
		h := srv.Handler()
		cookie, csrf := login(t, h)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPatch, "/api/v1/redis/users/admin", cookie, csrf, customBody))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != CodeProtectedResource {
			t.Fatalf("code = %q", body.Error.Code)
		}
		if len(mem.ACLSetUserCalls) != 0 {
			t.Fatalf("SETUSER on protected: %#v", mem.ACLSetUserCalls)
		}
	})
	t.Run("not_found", func(t *testing.T) {
		mem := &redisadmin.MemoryClient{ACLLines: []string{enableUserACLLine}}
		srv := testServerWithRedis(t, redisadmin.NewService(mem))
		seedOwner(t, srv)
		h := srv.Handler()
		cookie, csrf := login(t, h)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPatch, "/api/v1/redis/users/missing", cookie, csrf, customBody))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != CodeNotFound || body.Error.Message != "Not found" {
			t.Fatalf("error = %#v", body.Error)
		}
		if len(mem.ACLSetUserCalls) != 0 {
			t.Fatalf("SETUSER on missing: %#v", mem.ACLSetUserCalls)
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{
			ACLListErr: errors.New("dial tcp 10.0.0.1:6379 canary-secret"),
		}))
		seedOwner(t, srv)
		h := srv.Handler()
		cookie, csrf := login(t, h)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPatch, redisPatchPath, cookie, csrf, customBody))
		assertPatchUnavailable(t, rec)
		assertNoRedisUsersCanary(t, rec.Body.String())
	})
}

func TestRedisUserPatchCustomCategoriesUnknown(t *testing.T) {
	mem := &redisadmin.MemoryClient{ACLLines: []string{enableUserACLLine}}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPatch, redisPatchPath, cookie, csrf, `{"key_pattern":"project_a","preset":"custom","commands":["ping"],"categories":["@string"]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeValidationError {
		t.Fatalf("code = %q", body.Error.Code)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("SETUSER on unknown categories: %#v", mem.ACLSetUserCalls)
	}
}

func assertHTTPUpdateRulesSafe(t *testing.T, rules []string) {
	t.Helper()
	joined := strings.Join(rules, " ")
	for _, bad := range []string{"reset", "resetpass", "on", "off"} {
		for _, rule := range rules {
			if rule == bad {
				t.Fatalf("forbidden rule %q in %#v", bad, rules)
			}
		}
	}
	if strings.Contains(joined, ">") {
		t.Fatalf("forbidden password rule in %#v", rules)
	}
	if rules[0] != "resetkeys" || rules[3] != "nocommands" || rules[4] != "-@all" {
		t.Fatalf("SETUSER shape = %#v", rules)
	}
}

func assertHTTPCreateRulesSafe(t *testing.T, rules []string, password, pattern string, wantCmds []string) {
	t.Helper()
	if len(rules) < 6 {
		t.Fatalf("rules too short: %#v", rules)
	}
	joined := strings.Join(rules, " ")
	if rules[0] != "reset" || rules[1] != "on" {
		t.Fatalf("prefix rules = %#v", rules[:2])
	}
	if rules[2] != ">"+password {
		t.Fatalf("password rule = %q", rules[2])
	}
	if rules[3] != "~"+pattern {
		t.Fatalf("key rule = %q", rules[3])
	}
	if rules[4] != "resetchannels" || rules[5] != "-@all" {
		t.Fatalf("channel/category rules = %#v", rules[4:6])
	}
	for _, rule := range rules {
		if rule == "nocommands" {
			t.Fatalf("create SETUSER has nocommands: %#v", rules)
		}
	}
	upperJoined := strings.ToUpper(joined)
	for _, bad := range []string{"+@ALL", "+ACL", "+CONFIG", "+FLUSHALL", "+FLUSHDB", "+SCRIPT", "+EVAL"} {
		if strings.Contains(upperJoined, bad) {
			t.Fatalf("dangerous %s in %s", bad, joined)
		}
	}
	got := httpGrantedCommands(rules)
	if !stringSlicesEqual(got, wantCmds) {
		t.Fatalf("granted = %#v want %#v", got, wantCmds)
	}
}

func assertAuditUpdateOmitsCommands(t *testing.T, srv *Server) {
	t.Helper()
	var metadata string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events WHERE action = 'redis.user.update' ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata, `"commands"`) {
		t.Fatalf("audit leaked commands: %s", metadata)
	}
}
