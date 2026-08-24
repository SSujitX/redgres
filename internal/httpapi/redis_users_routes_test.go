package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/redisadmin"
)

const (
	aclCanaryHash = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	aclCanaryLine = "user antirez on #9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 >canary-secret ~objects:* &* +@all -@admin -@dangerous"
)

func TestRedisUsersRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/redis/users", nil))
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
	for _, key := range []string{"state", "users", "user", "reason"} {
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

func TestRedisUserDetailRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/redis/users/project_a", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"state", "users", "user", "reason"} {
		if _, ok := raw[key]; ok {
			t.Fatalf("401 leaked %s: %s", key, rec.Body.Bytes())
		}
	}
}

func TestRedisUsersNotConfigured(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/users", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["state"] != "not_configured" {
		t.Fatalf("body = %s", rec.Body.Bytes())
	}
	users, ok := raw["users"].([]any)
	if !ok || users == nil || len(users) != 0 {
		t.Fatalf("users = %#v", raw["users"])
	}
	if _, ok := raw["reason"]; ok {
		t.Fatalf("not_configured leaked reason: %s", rec.Body.Bytes())
	}
	if _, ok := raw["truncated"]; ok {
		t.Fatalf("not_configured leaked truncated: %s", rec.Body.Bytes())
	}
	if _, ok := raw["user"]; ok {
		t.Fatalf("list leaked user: %s", rec.Body.Bytes())
	}
	id, _ := raw["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
}

func TestRedisUserDetailNotConfigured(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/users/project_a", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["state"] != "not_configured" {
		t.Fatalf("body = %s", rec.Body.Bytes())
	}
	if _, ok := raw["user"]; ok {
		t.Fatalf("not_configured leaked user: %s", rec.Body.Bytes())
	}
	if _, ok := raw["users"]; ok {
		t.Fatalf("detail leaked users: %s", rec.Body.Bytes())
	}
	if _, ok := raw["reason"]; ok {
		t.Fatalf("not_configured leaked reason: %s", rec.Body.Bytes())
	}
}

func TestRedisUsersOKList(t *testing.T) {
	srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{
		ACLLines: []string{
			"user zebra on ~z:* -@all +ping",
			"user project_a on ~project_a:* -@all +ping",
			"user default on nopass ~* &* +@all",
		},
	}))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/users", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["state"] != "ok" {
		t.Fatalf("body = %s", rec.Body.Bytes())
	}
	if raw["truncated"] != false {
		t.Fatalf("truncated = %#v", raw["truncated"])
	}
	if _, ok := raw["reason"]; ok {
		t.Fatalf("ok leaked reason: %s", rec.Body.Bytes())
	}
	users, ok := raw["users"].([]any)
	if !ok || len(users) != 3 {
		t.Fatalf("users = %#v", raw["users"])
	}
	names := make([]string, 0, 3)
	for _, item := range users {
		row, _ := item.(map[string]any)
		names = append(names, row["username"].(string))
		if _, has := row["commands"]; has {
			t.Fatalf("list included commands: %s", rec.Body.Bytes())
		}
		if _, has := row["categories"]; has {
			t.Fatalf("list included categories: %s", rec.Body.Bytes())
		}
	}
	if strings.Join(names, ",") != "default,project_a,zebra" {
		t.Fatalf("order = %#v", names)
	}
	def, _ := users[0].(map[string]any)
	if def["protected"] != true || def["preset"] != "custom" || def["rule_fidelity"] != "limited" {
		t.Fatalf("default = %#v", def)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("audit events changed from %d to %d", before, after)
	}
}

func TestRedisUserDetailOK(t *testing.T) {
	srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{
		ACLLines: []string{"user project_a on ~project_a:* -@all +echo +get +ping"},
	}))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/users/project_a", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["state"] != "ok" {
		t.Fatalf("body = %s", rec.Body.Bytes())
	}
	user, ok := raw["user"].(map[string]any)
	if !ok {
		t.Fatalf("user missing: %s", rec.Body.Bytes())
	}
	if user["username"] != "project_a" || user["enabled"] != true || user["key_pattern"] != "project_a:*" {
		t.Fatalf("user = %#v", user)
	}
	if user["preset"] != "custom" || user["rule_fidelity"] != "exact" || user["protected"] != false {
		t.Fatalf("labels = %#v", user)
	}
	if _, ok := user["queue_kind"]; ok {
		t.Fatalf("queue_kind present: %#v", user)
	}
	cmds, _ := user["commands"].([]any)
	if fmt.Sprint(cmds) != "[echo get ping]" {
		t.Fatalf("commands = %#v", cmds)
	}
	cats, ok := user["categories"].([]any)
	if !ok || cats == nil || len(cats) != 0 {
		t.Fatalf("categories = %#v", user["categories"])
	}
}

func TestRedisUserDetailProtectedVisible(t *testing.T) {
	srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{
		ACLLines: []string{"user default on nopass ~* &* +@all"},
	}))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/users/default", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	user, _ := raw["user"].(map[string]any)
	if raw["state"] != "ok" || user["username"] != "default" || user["protected"] != true {
		t.Fatalf("body = %s", rec.Body.Bytes())
	}
}

func TestRedisUserDetailNotFound(t *testing.T) {
	srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{
		ACLLines: []string{"user project_a on ~project_a:* -@all +ping"},
	}))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/users/missing", cookie, csrf, ""))
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
	if strings.Contains(rec.Body.String(), `"user"`) && strings.Contains(rec.Body.String(), `"state"`) {
		t.Fatalf("404 looked like a user payload: %s", rec.Body.Bytes())
	}
}

func TestRedisUserDetailInvalidUsername(t *testing.T) {
	srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{
		ACLLines: []string{"user project_a on ~project_a:* -@all +ping"},
	}))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	cases := []string{
		"/api/v1/redis/users/bad.name",
		"/api/v1/redis/users/" + strings.Repeat("a", 65),
		"/api/v1/redis/users/%20",
	}
	for _, path := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, path, cookie, csrf, ""))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d body = %s", path, rec.Code, rec.Body.Bytes())
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != CodeValidationError {
			t.Fatalf("%s code = %q", path, body.Error.Code)
		}
		raw := rec.Body.String()
		if strings.Contains(raw, "bad.name") || strings.Contains(raw, strings.Repeat("a", 65)) || strings.Contains(raw, "%20") {
			t.Fatalf("400 echoed param: %s", raw)
		}
	}
}

func TestRedisUsersRejectsMutatingMethods(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, path := range []string{"/api/v1/redis/users", "/api/v1/redis/users/project_a"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, authed(method, path, cookie, csrf, "{}"))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s status = %d body = %s", method, path, rec.Code, rec.Body.Bytes())
			}
			var body errorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != CodeMethodNotAllowed {
				t.Fatalf("%s %s code = %q", method, path, body.Error.Code)
			}
		}
	}
}

func TestRedisUsersCanaryAbsent(t *testing.T) {
	srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{
		ACLLines: []string{aclCanaryLine, "user project_a on >canary-secret ~project_a:* -@all +ping"},
	}))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	for _, path := range []string{"/api/v1/redis/users", "/api/v1/redis/users/antirez", "/api/v1/redis/users/project_a"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, path, cookie, csrf, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body = %s", path, rec.Code, rec.Body.Bytes())
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s Cache-Control = %q", path, rec.Header().Get("Cache-Control"))
		}
		assertNoRedisUsersCanary(t, rec.Body.String())
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("audit events changed from %d to %d", before, after)
	}
}

func TestRedisUsersUnavailableReasons(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		reason string
	}{
		{name: "auth_failed", err: errors.New("NOAUTH Authentication required. canary-secret"), reason: "auth_failed"},
		{name: "permission_denied", err: errors.New("NOPERM acl|list canary-secret"), reason: "permission_denied"},
		{name: "unreachable", err: errors.New("dial tcp 10.0.0.1:6379 canary-secret"), reason: "unreachable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{ACLListErr: tc.err}))
			seedOwner(t, srv)
			h := srv.Handler()
			cookie, csrf := login(t, h)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/users", cookie, csrf, ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
			}
			assertNoRedisUsersCanary(t, rec.Body.String())
			var raw map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			if raw["state"] != "unavailable" || raw["reason"] != tc.reason {
				t.Fatalf("body = %s", rec.Body.Bytes())
			}
			if _, ok := raw["users"]; ok {
				t.Fatalf("unavailable leaked users: %s", rec.Body.Bytes())
			}
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/users/project_a", cookie, csrf, ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("detail status = %d body = %s", rec.Code, rec.Body.Bytes())
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			if raw["state"] != "unavailable" || raw["reason"] != tc.reason {
				t.Fatalf("detail body = %s", rec.Body.Bytes())
			}
			if _, ok := raw["user"]; ok {
				t.Fatalf("unavailable leaked user: %s", rec.Body.Bytes())
			}
		})
	}
}

func TestRedisUsersDoesNotChangeStatusRoutes(t *testing.T) {
	srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{InfoText: sampleRedisInfo, Size: 50}))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/status", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got := decodeStatus(t, rec)
	if got["redis"].state != "ok" || got["redis"].reason != "" {
		t.Fatalf("GET /status redis = %#v", got["redis"])
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/status", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("redis/status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["state"] != "ok" {
		t.Fatalf("redis/status body = %s", rec.Body.Bytes())
	}
	if _, ok := raw["users"]; ok {
		t.Fatalf("redis/status leaked users: %s", rec.Body.Bytes())
	}
	metrics, ok := raw["metrics"].(map[string]any)
	if !ok || metrics["version"] != "8.2.1" || metrics["db_size"] != float64(50) {
		t.Fatalf("metrics = %#v", raw["metrics"])
	}
}

func assertNoRedisUsersCanary(t *testing.T, raw string) {
	t.Helper()
	for _, leak := range []string{
		aclCanaryHash,
		"#" + aclCanaryHash,
		"canary-secret",
		">canary-secret",
		"acl_rule",
		"nopass",
		"NOAUTH",
		"WRONGPASS",
		"NOPERM",
		"password=",
	} {
		if strings.Contains(raw, leak) {
			t.Fatalf("leaked %q in %s", leak, raw)
		}
	}
}
