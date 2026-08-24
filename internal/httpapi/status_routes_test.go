package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
)

func TestStatusRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	raw := rec.Body.String()
	if strings.Contains(raw, `"components"`) {
		t.Fatalf("401 leaked components: %s", raw)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeUnauthorized {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestStatusDefaultPostgresNotConfigured(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/status", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var body struct {
		Components []struct {
			ID     string `json:"id"`
			State  string `json:"state"`
			Reason string `json:"reason"`
		} `json:"components"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !requestIDOK(body.RequestID) {
		t.Fatalf("request_id = %q", body.RequestID)
	}
	want := []struct{ id, state string }{
		{"redgres_state", "ok"},
		{"postgres_direct", "not_configured"},
		{"pgbouncer", "not_implemented"},
		{"redis", "not_implemented"},
		{"tool_links", "not_configured"},
	}
	if len(body.Components) != 5 {
		t.Fatalf("components = %#v", body.Components)
	}
	for i, item := range want {
		got := body.Components[i]
		if got.ID != item.id || got.State != item.state || got.Reason != "" {
			t.Fatalf("index %d = %#v want %+v", i, got, item)
		}
	}
}

func TestStatusPostgresUnavailableKeepsRedis(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{PingErr: postgresadmin.ErrUnavailable}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/status", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got := decodeStatus(t, rec)
	if got["redgres_state"].state != "ok" || got["redgres_state"].reason != "" {
		t.Fatalf("state = %#v", got["redgres_state"])
	}
	if got["postgres_direct"].state != "unavailable" || got["postgres_direct"].reason != "unreachable" {
		t.Fatalf("postgres = %#v", got["postgres_direct"])
	}
	if got["redis"].state != "not_implemented" {
		t.Fatalf("redis = %#v", got["redis"])
	}
}

func TestStatusHealthyPostgresKeepsRedisNotImplemented(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/status", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got := decodeStatus(t, rec)
	if got["postgres_direct"].state != "ok" || got["postgres_direct"].reason != "" {
		t.Fatalf("postgres = %#v", got["postgres_direct"])
	}
	if got["redis"].state != "not_implemented" {
		t.Fatalf("redis = %#v", got["redis"])
	}
}

func TestStatusRejectsMutatingMethods(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(method, "/api/v1/status", cookie, csrf, "{}"))
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

func TestStatusOmitsCanarySecrets(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{PingErr: errors.New("password=canary-secret host=10.0.0.1")}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/status", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "canary-secret") || strings.Contains(raw, "10.0.0.1") || strings.Contains(raw, "password=") {
		t.Fatalf("leaked canary: %s", raw)
	}
}

func TestHealthzUnchangedWithoutComponents(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var body healthzBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q", body.Status)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, `"components"`) {
		t.Fatalf("healthz included components: %s", raw)
	}
}

type statusComponent struct {
	id, state, reason string
}

func decodeStatus(t *testing.T, rec *httptest.ResponseRecorder) map[string]statusComponent {
	t.Helper()
	var body struct {
		Components []struct {
			ID     string `json:"id"`
			State  string `json:"state"`
			Reason string `json:"reason"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := make(map[string]statusComponent, len(body.Components))
	for _, item := range body.Components {
		out[item.ID] = statusComponent{id: item.ID, state: item.State, reason: item.Reason}
	}
	return out
}
