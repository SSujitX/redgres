package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/redisadmin"
)

func TestRedisPresetsRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/redis/presets", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"presets", "state", "users", "user", "reason", "credential"} {
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

func TestRedisPresetsMethodNotAllowed(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(method, "/api/v1/redis/presets", cookie, csrf, "{}"))
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

func TestRedisPresetsCatalogEqualsCreateGrants(t *testing.T) {
	mem := &redisadmin.MemoryClient{}
	srv := testServerWithRedis(t, redisadmin.NewService(mem))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/presets", cookie, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("GET presets wrote audit: %d -> %d", before, after)
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("GET presets used SETUSER: %#v", mem.ACLSetUserCalls)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["state"]; ok {
		t.Fatalf("presets leaked state: %s", rec.Body.Bytes())
	}
	if _, ok := raw["reason"]; ok {
		t.Fatalf("presets leaked reason: %s", rec.Body.Bytes())
	}
	if strings.Contains(rec.Body.String(), `"custom"`) {
		t.Fatalf("catalog includes custom: %s", rec.Body.Bytes())
	}
	presets, ok := raw["presets"].([]any)
	if !ok {
		t.Fatalf("presets missing or null: %s", rec.Body.Bytes())
	}
	id, _ := raw["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
	want := redisadmin.NamedPresets()
	if len(presets) != len(want) {
		t.Fatalf("len = %d want %d", len(presets), len(want))
	}
	creates := []struct {
		body string
	}{
		{`{"username":"cat_crw","key_pattern":"cat_crw","preset":"cache-read-write"}`},
		{`{"username":"cat_ro","key_pattern":"cat_ro","preset":"read-only"}`},
		{`{"username":"cat_ql","key_pattern":"cat_ql","preset":"queue-worker","queue_kind":"lists"}`},
		{`{"username":"cat_qs","key_pattern":"cat_qs","preset":"queue-worker","queue_kind":"streams"}`},
		{`{"username":"cat_qz","key_pattern":"cat_qz","preset":"queue-worker","queue_kind":"sorted-sets"}`},
	}
	for i, tc := range creates {
		item, _ := presets[i].(map[string]any)
		if item["preset"] != want[i].Preset {
			t.Fatalf("[%d] preset = %#v", i, item["preset"])
		}
		if want[i].QueueKind == "" {
			if _, ok := item["queue_kind"]; ok {
				t.Fatalf("[%d] queue_kind present: %#v", i, item)
			}
		} else if item["queue_kind"] != want[i].QueueKind {
			t.Fatalf("[%d] queue_kind = %#v", i, item["queue_kind"])
		}
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/redis/users", cookie, csrf, tc.body))
		if rec.Code != http.StatusCreated {
			t.Fatalf("[%d] create status = %d body = %s", i, rec.Code, rec.Body.Bytes())
		}
		if len(mem.ACLSetUserCalls) != i+1 {
			t.Fatalf("[%d] SETUSER calls = %d", i, len(mem.ACLSetUserCalls))
		}
		gotCmds := httpGrantedCommands(mem.ACLSetUserCalls[i].Rules)
		catalogCmds := jsonStringSlice(item["commands"])
		if !stringSlicesEqual(gotCmds, catalogCmds) || !stringSlicesEqual(catalogCmds, want[i].Commands) {
			t.Fatalf("[%d] catalog/create mismatch catalog=%#v grants=%#v named=%#v", i, catalogCmds, gotCmds, want[i].Commands)
		}
	}
}

func TestRedisPresetsNilAdapterOK(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/presets", cookie, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	presets, ok := raw["presets"].([]any)
	if !ok || len(presets) != 5 {
		t.Fatalf("presets = %#v", raw["presets"])
	}
	if _, ok := raw["state"]; ok {
		t.Fatalf("nil adapter leaked state: %s", rec.Body.Bytes())
	}
}

func TestRedisPresetsDoesNotUseRedisClient(t *testing.T) {
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
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/presets", cookie, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if len(mem.ACLSetUserCalls) != 0 {
		t.Fatalf("SETUSER calls = %#v", mem.ACLSetUserCalls)
	}
	if after := countAuditEvents(t, srv); after != before {
		t.Fatalf("GET presets wrote audit")
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["presets"].([]any); !ok {
		t.Fatalf("presets missing: %s", rec.Body.Bytes())
	}
}

func httpGrantedCommands(rules []string) []string {
	out := make([]string, 0)
	for _, rule := range rules {
		if !strings.HasPrefix(rule, "+") {
			continue
		}
		cmd := strings.ToLower(strings.TrimPrefix(rule, "+"))
		if cmd == "" || strings.HasPrefix(cmd, "@") {
			continue
		}
		out = append(out, cmd)
	}
	return sortedUniqueHTTP(out)
}

func jsonStringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			continue
		}
		out = append(out, s)
	}
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sortedUniqueHTTP(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
