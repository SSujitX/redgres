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

func TestRedisUsersCreateCustomStill400(t *testing.T) {
	mem := &redisadmin.MemoryClient{}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	cases := []struct {
		name  string
		body  string
		field string
	}{
		{name: "preset-custom", body: `{"username":"project_a","key_pattern":"project_a","preset":"custom"}`, field: "preset"},
		{name: "commands-unknown", body: `{"username":"project_a","key_pattern":"project_a","preset":"custom","commands":["get"]}`},
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
			if body.Error.Code != CodeValidationError {
				t.Fatalf("code = %q", body.Error.Code)
			}
			if tc.field != "" && body.Error.Fields[tc.field] == "" {
				t.Fatalf("missing fields.%s: %#v", tc.field, body.Error.Fields)
			}
		})
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("SETUSER on create custom: %#v", mem.ACLSetUserCalls)
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
