package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/redisadmin"
)

const sampleRedisInfo = `# Server
redis_version:8.2.1
uptime_in_seconds:123
# Clients
connected_clients:4
# Memory
used_memory:1048576
maxmemory:0
# Stats
instantaneous_ops_per_sec:12
`

const redisStatusCanary = "rediss://:canary-secret@10.0.0.1:6379/0?skip_verify=true"

func assertNoRedisStatusCanary(t *testing.T, raw string) {
	t.Helper()
	for _, leak := range []string{"canary-secret", "10.0.0.1", redisStatusCanary, "NOAUTH", "WRONGPASS", "NOPERM", "password="} {
		if strings.Contains(raw, leak) {
			t.Fatalf("leaked %q in %s", leak, raw)
		}
	}
}

func TestRedisStatusRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/redis/status", nil))
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
	for _, key := range []string{"state", "metrics", "reason"} {
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

func TestRedisStatusNotConfigured(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/status", cookie, csrf, ""))
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
	if _, ok := raw["metrics"]; ok {
		t.Fatalf("not_configured leaked metrics: %s", rec.Body.Bytes())
	}
	if _, ok := raw["reason"]; ok {
		t.Fatalf("not_configured leaked reason: %s", rec.Body.Bytes())
	}
	id, _ := raw["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
}

func TestRedisStatusOKAllMetricKeys(t *testing.T) {
	srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{InfoText: sampleRedisInfo, Size: 50}))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/status", cookie, csrf, ""))
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
	if _, ok := raw["reason"]; ok {
		t.Fatalf("ok leaked reason: %s", rec.Body.Bytes())
	}
	metrics, ok := raw["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("metrics missing: %s", rec.Body.Bytes())
	}
	want := map[string]any{
		"version":           "8.2.1",
		"uptime_seconds":    float64(123),
		"connected_clients": float64(4),
		"used_memory_bytes": float64(1048576),
		"max_memory_bytes":  float64(0),
		"ops_per_sec":       float64(12),
		"db_size":           float64(50),
	}
	for key, value := range want {
		got, present := metrics[key]
		if !present || got != value {
			t.Fatalf("metrics[%s] = %#v want %#v body = %s", key, got, value, rec.Body.Bytes())
		}
	}
	if _, ok := metrics["latency_ms"].(float64); !ok {
		t.Fatalf("latency_ms missing or not a number: %#v", metrics["latency_ms"])
	}
	if len(metrics) != 8 {
		t.Fatalf("metrics keys = %#v", metrics)
	}
	id, _ := raw["request_id"].(string)
	if !requestIDOK(id) {
		t.Fatalf("request_id = %q", id)
	}
}

func TestRedisStatusReasons(t *testing.T) {
	cases := []struct {
		name   string
		client *redisadmin.MemoryClient
		reason string
	}{
		{
			name:   "auth_failed",
			client: &redisadmin.MemoryClient{PingErr: errors.New("NOAUTH Authentication required. " + redisStatusCanary)},
			reason: "auth_failed",
		},
		{
			name:   "permission_denied",
			client: &redisadmin.MemoryClient{PingErr: errors.New("NOPERM this user has no permissions to run the 'ping' command " + redisStatusCanary)},
			reason: "permission_denied",
		},
		{
			name:   "unreachable",
			client: &redisadmin.MemoryClient{PingErr: errors.New("dial tcp 10.0.0.1:6379: connect: connection refused " + redisStatusCanary)},
			reason: "unreachable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := testServerWithRedis(t, redisadmin.NewService(tc.client))
			seedOwner(t, srv)
			h := srv.Handler()
			cookie, csrf := login(t, h)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/status", cookie, csrf, ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
			}
			assertNoRedisStatusCanary(t, rec.Body.String())
			var raw map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			if raw["state"] != "unavailable" || raw["reason"] != tc.reason {
				t.Fatalf("body = %s", rec.Body.Bytes())
			}
			if _, ok := raw["metrics"]; ok {
				t.Fatalf("unavailable leaked metrics: %s", rec.Body.Bytes())
			}
		})
	}
}

func TestRedisStatusCanaryAbsent(t *testing.T) {
	srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{
		InfoErr: errors.New("NOPERM info " + redisStatusCanary),
	}))
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	before := countAuditEvents(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/redis/status", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	assertNoRedisStatusCanary(t, rec.Body.String())
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	after := countAuditEvents(t, srv)
	if after != before {
		t.Fatalf("audit events changed from %d to %d", before, after)
	}
}

func TestRedisStatusRejectsMutatingMethods(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(method, "/api/v1/redis/status", cookie, csrf, "{}"))
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

func TestRedisStatusHealthzUnchanged(t *testing.T) {
	srv := testServerWithRedis(t, redisadmin.NewService(&redisadmin.MemoryClient{InfoText: sampleRedisInfo, Size: 50}))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	raw := rec.Body.String()
	if strings.Contains(raw, `"components"`) || strings.Contains(raw, `"metrics"`) || strings.Contains(raw, `"state"`) {
		t.Fatalf("healthz leaked redis status keys: %s", raw)
	}
}

func countAuditEvents(t *testing.T, srv *Server) int {
	t.Helper()
	var n int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
