package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/SSujitX/redgres/internal/redisadmin"
)

type listPanicCatalog struct {
	postgresadmin.MemoryCatalog
}

func (c *listPanicCatalog) List(context.Context) ([]postgresadmin.CatalogRow, error) {
	panic("List must not be called")
}

type searchBody struct {
	Groups []struct {
		ID        string `json:"id"`
		Label     string `json:"label"`
		Service   string `json:"service"`
		Status    string `json:"status"`
		Truncated bool   `json:"truncated"`
		Hits      []struct {
			ID    string `json:"id"`
			Type  string `json:"type"`
			Label string `json:"label"`
			Owner string `json:"owner"`
		} `json:"hits"`
	} `json:"groups"`
	Limit     int    `json:"limit"`
	RequestID string `json:"request_id"`
}

func decodeSearch(t *testing.T, rec *httptest.ResponseRecorder) searchBody {
	t.Helper()
	var body searchBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestSearchRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=a", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	raw := rec.Body.String()
	if strings.Contains(raw, `"groups"`) {
		t.Fatalf("401 leaked groups: %s", raw)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeUnauthorized {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestSearchRejectsMissingQueryWithoutListing(t *testing.T) {
	svc := postgresadmin.NewService(&listPanicCatalog{}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, path := range []string{"/api/v1/search", "/api/v1/search?q=", "/api/v1/search?q=%20"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, path, cookie, csrf, ""))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d body = %s", path, rec.Code, rec.Body.Bytes())
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != CodeValidationError || body.Error.Fields["q"] != "too_short" {
			t.Fatalf("%s fields = %+v", path, body.Error)
		}
	}
}

func TestSearchRejectsLongQueryWithoutListing(t *testing.T) {
	svc := postgresadmin.NewService(&listPanicCatalog{}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	q := strings.Repeat("x", 129)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q="+q, cookie, csrf, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeValidationError || body.Error.Fields["q"] != "too_long" {
		t.Fatalf("fields = %+v", body.Error)
	}
	if strings.Contains(rec.Body.String(), q) {
		t.Fatalf("echoed q: %s", rec.Body.String())
	}
}

func TestSearchRejectsNonIntegerLimit(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=a&limit=abc", cookie, csrf, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeValidationError || body.Error.Fields["limit"] == "" {
		t.Fatalf("fields = %+v", body.Error)
	}
}

func TestSearchClampsLimit(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, path := range []string{"/api/v1/search?q=a&limit=0", "/api/v1/search?q=a&limit=99"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, path, cookie, csrf, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body = %s", path, rec.Code, rec.Body.Bytes())
		}
		got := decodeSearch(t, rec)
		if got.Limit != 20 {
			t.Fatalf("%s limit = %d", path, got.Limit)
		}
	}
}

func TestSearchNilPostgresIsNotConfigured(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=a", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	got := decodeSearch(t, rec)
	if !requestIDOK(got.RequestID) {
		t.Fatalf("request_id = %q", got.RequestID)
	}
	if len(got.Groups) != 2 {
		t.Fatalf("groups = %#v", got.Groups)
	}
	if got.Groups[0].ID != "postgres_databases" || got.Groups[0].Status != "not_configured" || len(got.Groups[0].Hits) != 0 {
		t.Fatalf("postgres = %#v", got.Groups[0])
	}
	if got.Groups[1].ID != "redis_acl_users" || got.Groups[1].Status != "not_configured" || len(got.Groups[1].Hits) != 0 {
		t.Fatalf("redis = %#v", got.Groups[1])
	}
	if got.Limit != 20 {
		t.Fatalf("limit = %d", got.Limit)
	}
}

func TestSearchOmitsProtectedAndReturnsManageableHit(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "postgres", Owner: "postgres", AllowConn: true},
		{Name: "project_a", Owner: "project_a_role", AllowConn: true},
	}}, postgresadmin.NewPolicy(config.Config{PostgresDatabase: "postgres"}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=postgres", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got := decodeSearch(t, rec)
	if got.Groups[0].Status != "ok" {
		t.Fatalf("status = %#v", got.Groups[0])
	}
	for _, hit := range got.Groups[0].Hits {
		if hit.Label == "postgres" || strings.HasSuffix(hit.ID, ":postgres") {
			t.Fatalf("protected hit = %#v", hit)
		}
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=project", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got = decodeSearch(t, rec)
	if len(got.Groups[0].Hits) != 1 {
		t.Fatalf("hits = %#v", got.Groups[0].Hits)
	}
	hit := got.Groups[0].Hits[0]
	if hit.ID != "postgres_database:project_a" || hit.Type != "postgres_database" || hit.Label != "project_a" {
		t.Fatalf("hit = %#v", hit)
	}
	if hit.Owner != "" {
		t.Fatalf("owner leaked: %#v", hit)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "saved_credential") || strings.Contains(raw, `"owner"`) {
		t.Fatalf("leaked inventory fields: %s", raw)
	}
}

func TestSearchUnavailablePostgresKeepsRedisGroup(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Err: errors.New("postgresql://canary:secret@10.0.0.1/db")}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=a", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got := decodeSearch(t, rec)
	if got.Groups[0].Status != "unavailable" || len(got.Groups[0].Hits) != 0 {
		t.Fatalf("postgres = %#v", got.Groups[0])
	}
	if got.Groups[1].Status != "not_configured" || len(got.Groups[1].Hits) != 0 {
		t.Fatalf("redis = %#v", got.Groups[1])
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "canary") || strings.Contains(raw, "secret") || strings.Contains(raw, "10.0.0.1") {
		t.Fatalf("leaked canary: %s", raw)
	}
}

func TestSearchRejectsMutatingMethods(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(method, "/api/v1/search?q=a", cookie, csrf, "{}"))
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

func TestSearchOmitsForbiddenKeys(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "project_a", Owner: "project_a_role", AllowConn: true},
	}}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=project", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	raw := rec.Body.String()
	for _, key := range []string{`"events"`, `"metadata"`, `"password"`, `"csrf_token"`} {
		if strings.Contains(raw, key) {
			t.Fatalf("forbidden key %s in %s", key, raw)
		}
	}
}

func TestSearchNilRedisIsNotConfigured(t *testing.T) {
	svc := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "project_a", Owner: "project_a_role", AllowConn: true},
	}}, postgresadmin.NewPolicy(config.Config{}))
	srv := testServerWithPostgres(t, svc)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=project", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got := decodeSearch(t, rec)
	if got.Groups[1].ID != "redis_acl_users" || got.Groups[1].Status != "not_configured" {
		t.Fatalf("redis = %#v", got.Groups[1])
	}
	if got.Groups[1].Hits == nil || len(got.Groups[1].Hits) != 0 {
		t.Fatalf("hits = %#v", got.Groups[1].Hits)
	}
	if strings.Contains(rec.Body.String(), `"reason"`) {
		t.Fatalf("search group leaked reason: %s", rec.Body.String())
	}
}

func TestSearchNilPostgresStillProbesRedis(t *testing.T) {
	srv, _ := testServer(t, nil)
	srv.redis = redisadmin.NewService(&redisadmin.MemoryClient{
		ACLLines: []string{"user project_a on ~project_a:* -@all +ping"},
	})
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=project", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got := decodeSearch(t, rec)
	if got.Groups[0].Status != "not_configured" || len(got.Groups[0].Hits) != 0 {
		t.Fatalf("postgres = %#v", got.Groups[0])
	}
	if got.Groups[1].Status != "ok" || len(got.Groups[1].Hits) != 1 {
		t.Fatalf("redis = %#v", got.Groups[1])
	}
	hit := got.Groups[1].Hits[0]
	if hit.ID != "redis_acl_user:project_a" || hit.Type != "redis_acl_user" || hit.Label != "project_a" {
		t.Fatalf("hit = %#v", hit)
	}
}

func TestSearchMemoryACLUserHit(t *testing.T) {
	pg := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "project_a", Owner: "project_a_role", AllowConn: true},
	}}, postgresadmin.NewPolicy(config.Config{}))
	rd := redisadmin.NewService(&redisadmin.MemoryClient{
		ACLLines: []string{"user project_a on ~project_a:* -@all +ping"},
	})
	srv := testServerWithPostgres(t, pg)
	srv.redis = rd
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=project", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got := decodeSearch(t, rec)
	if got.Groups[1].Status != "ok" || len(got.Groups[1].Hits) != 1 {
		t.Fatalf("redis = %#v", got.Groups[1])
	}
	hit := got.Groups[1].Hits[0]
	if hit.ID != "redis_acl_user:project_a" || hit.Type != "redis_acl_user" || hit.Label != "project_a" || hit.Owner != "" {
		t.Fatalf("hit = %#v", hit)
	}

	statusRec := httptest.NewRecorder()
	h.ServeHTTP(statusRec, authed(http.MethodGet, "/api/v1/status", cookie, csrf, ""))
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", statusRec.Code, statusRec.Body.Bytes())
	}
	usersRec := httptest.NewRecorder()
	h.ServeHTTP(usersRec, authed(http.MethodGet, "/api/v1/redis/users", cookie, csrf, ""))
	if usersRec.Code != http.StatusOK {
		t.Fatalf("users = %d body = %s", usersRec.Code, usersRec.Body.Bytes())
	}
}

func TestSearchOmitsProtectedRedisUser(t *testing.T) {
	rd := redisadmin.NewService(&redisadmin.MemoryClient{
		ACLLines: []string{
			"user default on nopass ~* &* +@all",
			"user project_a on ~project_a:* -@all +ping",
		},
	})
	srv, _ := testServer(t, nil)
	srv.redis = rd
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=default", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got := decodeSearch(t, rec)
	if got.Groups[1].Status != "ok" {
		t.Fatalf("redis = %#v", got.Groups[1])
	}
	for _, hit := range got.Groups[1].Hits {
		if hit.Label == "default" || strings.HasSuffix(hit.ID, ":default") {
			t.Fatalf("protected hit = %#v", hit)
		}
	}

	usersRec := httptest.NewRecorder()
	h.ServeHTTP(usersRec, authed(http.MethodGet, "/api/v1/redis/users", cookie, csrf, ""))
	if usersRec.Code != http.StatusOK {
		t.Fatalf("users = %d body = %s", usersRec.Code, usersRec.Body.Bytes())
	}
	if !strings.Contains(usersRec.Body.String(), `"username":"default"`) {
		t.Fatalf("list omitted protected user: %s", usersRec.Body.String())
	}
}

func TestSearchACLAuthFailedKeepsPostgresHits(t *testing.T) {
	pg := postgresadmin.NewService(&postgresadmin.MemoryCatalog{Rows: []postgresadmin.CatalogRow{
		{Name: "project_a", Owner: "project_a_role", AllowConn: true},
	}}, postgresadmin.NewPolicy(config.Config{}))
	rd := redisadmin.NewService(&redisadmin.MemoryClient{
		ACLListErr: errors.New("NOAUTH Authentication required. password=canary-secret host=10.0.0.1"),
	})
	srv := testServerWithPostgres(t, pg)
	srv.redis = rd
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=project", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got := decodeSearch(t, rec)
	if got.Groups[0].Status != "ok" || len(got.Groups[0].Hits) != 1 {
		t.Fatalf("postgres = %#v", got.Groups[0])
	}
	if got.Groups[1].Status != "unavailable" || len(got.Groups[1].Hits) != 0 {
		t.Fatalf("redis = %#v", got.Groups[1])
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "canary-secret") || strings.Contains(raw, "NOAUTH") || strings.Contains(raw, `"reason"`) {
		t.Fatalf("leaked auth failure: %s", raw)
	}
}

func TestSearchOmitsACLHashAndPasswordCanaries(t *testing.T) {
	rd := redisadmin.NewService(&redisadmin.MemoryClient{ACLLines: []string{aclCanaryLine}})
	srv, _ := testServer(t, nil)
	srv.redis = rd
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/search?q=antirez", cookie, csrf, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	got := decodeSearch(t, rec)
	if got.Groups[1].Status != "ok" || len(got.Groups[1].Hits) != 1 || got.Groups[1].Hits[0].Label != "antirez" {
		t.Fatalf("redis = %#v", got.Groups[1])
	}
	raw := rec.Body.String()
	if strings.Contains(raw, aclCanaryHash) || strings.Contains(raw, "canary-secret") || strings.Contains(raw, ">canary") || strings.Contains(raw, "#9f86") {
		t.Fatalf("leaked ACL secret material: %s", raw)
	}
}
