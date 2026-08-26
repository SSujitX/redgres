package integration

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestLiveRedisMetricsAndRotationEligible verifies REDIS-001 live metrics
// against a real Redis (GET /api/v1/redis/status: version, uptime, clients,
// used/max memory, ops/s, DB size, latency) and PG-012 rotation_eligible for
// a vaulted LOGIN project owner in the live security overview.
func TestLiveRedisMetricsAndRotationEligible(t *testing.T) {
	h, cookie, csrf, _, _, _ := buildLiveHTTPServer(t, nil)

	// REDIS-001: live metrics over the real INFO parser.
	rec := liveAuthed(t, h, http.MethodGet, "/api/v1/redis/status", cookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("redis status code %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Metrics *struct {
			Version          string  `json:"version"`
			UptimeSeconds    int64   `json:"uptime_seconds"`
			ConnectedClients int64   `json:"connected_clients"`
			UsedMemoryBytes  int64   `json:"used_memory_bytes"`
			MaxMemoryBytes   int64   `json:"max_memory_bytes"`
			OpsPerSec        int64   `json:"ops_per_sec"`
			DBSize           int64   `json:"db_size"`
			LatencyMS        float64 `json:"latency_ms"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Metrics == nil {
		t.Fatal("missing metrics")
	}
	if body.Metrics.Version == "" {
		t.Fatal("empty redis version")
	}
	if body.Metrics.UptimeSeconds <= 0 {
		t.Fatalf("uptime_seconds = %d want > 0", body.Metrics.UptimeSeconds)
	}
	if body.Metrics.ConnectedClients < 1 {
		t.Fatalf("connected_clients = %d want >= 1", body.Metrics.ConnectedClients)
	}
	if body.Metrics.UsedMemoryBytes <= 0 {
		t.Fatalf("used_memory_bytes = %d want > 0", body.Metrics.UsedMemoryBytes)
	}
	if body.Metrics.MaxMemoryBytes < 0 {
		t.Fatalf("max_memory_bytes = %d want >= 0", body.Metrics.MaxMemoryBytes)
	}
	if body.Metrics.OpsPerSec < 0 || body.Metrics.DBSize < 0 {
		t.Fatalf("ops/s=%d db_size=%d want >= 0", body.Metrics.OpsPerSec, body.Metrics.DBSize)
	}
	if body.Metrics.LatencyMS <= 0 {
		t.Fatalf("latency_ms = %v want > 0", body.Metrics.LatencyMS)
	}

	// PG-012: a vaulted LOGIN project owner is rotation-eligible.
	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases", cookie, csrf,
		`{"database":"rot_live","owner":"app_rot_live"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create db status %d: %s", rec.Code, rec.Body.String())
	}
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/postgres/security", cookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("security status %d: %s", rec.Code, rec.Body.String())
	}
	var sec struct {
		Databases []struct {
			Name             string `json:"name"`
			RotationEligible bool   `json:"rotation_eligible"`
			Protected        bool   `json:"protected"`
		} `json:"databases"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sec); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, db := range sec.Databases {
		if db.Name == "rot_live" {
			found = true
			if db.Protected || !db.RotationEligible {
				t.Fatalf("rot_live protected=%v rotation_eligible=%v want false/true", db.Protected, db.RotationEligible)
			}
		}
	}
	if !found {
		t.Fatal("rot_live missing from security overview")
	}
}
