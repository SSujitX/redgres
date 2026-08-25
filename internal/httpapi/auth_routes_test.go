package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/auth"
)

const ownerPassword = "owner-secret-15"

func seedOwner(t *testing.T, srv *Server) {
	t.Helper()
	if _, err := auth.CreateOrReplaceOwner(srv.db, "admin", ownerPassword, false); err != nil {
		t.Fatal(err)
	}
}

func loginRequest(password string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"`+password+`"}`))
	req.Header.Set("Origin", "http://127.0.0.1:8790")
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	return req
}

func login(t *testing.T, h http.Handler) (cookie, csrf string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loginRequest(ownerPassword))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	csrf, _ = body["csrf_token"].(string)
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			cookie = c.Value
		}
	}
	if cookie == "" || csrf == "" {
		t.Fatal("missing session cookie or csrf")
	}
	return cookie, csrf
}

func authed(method, path, cookie, csrf, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	}
	if csrf != "" {
		req.Header.Set(csrfHeader, csrf)
	}
	req.Header.Set("Origin", "http://127.0.0.1:8790")
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestLoginSetsCookieFlags(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, loginRequest(ownerPassword))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store, max-age=0" {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("pragma = %q", rec.Header().Get("Pragma"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	owner, _ := body["owner"].(map[string]any)
	if owner["username"] != "admin" {
		t.Fatalf("owner = %#v", body["owner"])
	}
	if _, ok := body["csrf_token"].(string); !ok {
		t.Fatal("missing csrf_token")
	}
	if _, ok := body["tool_links"]; ok {
		t.Fatal("login included tool_links")
	}
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name != sessionCookieName {
			continue
		}
		found = true
		if !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteStrictMode {
			t.Fatalf("cookie flags %+v", c)
		}
		if c.Secure {
			t.Fatal("Secure must be false in development fixture")
		}
		if len(c.Value) != 64 {
			t.Fatalf("cookie value length %d", len(c.Value))
		}
	}
	if !found {
		t.Fatal("missing session cookie")
	}
}

func TestLoginGenericErrorAndRateLimit(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	var last errorBody
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, loginRequest("wrong-password-x"))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("fail %d status %d %s", i, rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &last); err != nil {
			t.Fatal(err)
		}
		if last.Error.Message != loginGenericMessage || last.Error.Code != CodeUnauthorized {
			t.Fatalf("generic = %+v", last.Error)
		}
		if strings.Contains(rec.Body.String(), "wrong-password") {
			t.Fatal("password leaked")
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loginRequest("wrong-password-x"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("lockout status %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeRateLimited {
		t.Fatalf("code = %q", body.Error.Code)
	}

	unknown := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"nobody","password":"wrong-password-x"}`))
	req.Header.Set("Origin", "http://127.0.0.1:8790")
	req.RemoteAddr = "10.0.0.8:9"
	h.ServeHTTP(unknown, req)
	if unknown.Code != http.StatusUnauthorized {
		t.Fatalf("unknown user %d", unknown.Code)
	}
}

func TestLoginRejectsBadOriginUnknownFieldAndBootstrap(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()

	rec := httptest.NewRecorder()
	req := loginRequest(ownerPassword)
	req.Header.Set("Origin", "https://evil.example")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("evil origin %d", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeCSRFInvalid {
		t.Fatalf("code = %q", body.Error.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"`+ownerPassword+`","extra":true}`))
	req.Header.Set("Origin", "http://127.0.0.1:8790")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(
		`{"username":"admin","password":"`+ownerPassword+`"} {"username":"admin","password":"`+ownerPassword+`"}`,
	))
	req.Header.Set("Origin", "http://127.0.0.1:8790")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON value %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", strings.NewReader(`{}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bootstrap %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET login %d", rec.Code)
	}
}

func TestSessionAndLogout(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, csrf := login(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/session", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthed session %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store, max-age=0" {
		t.Fatal("session cache")
	}
	if rec.Header().Get("Pragma") != "no-cache" {
		t.Fatal("session pragma")
	}
	var sess map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	rotated, _ := sess["csrf_token"].(string)
	if rotated == "" || rotated == csrf {
		t.Fatal("csrf was not rotated")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/auth/logout", cookie, "", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("logout without csrf %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	badOrigin := authed(http.MethodPost, "/api/v1/auth/logout", cookie, rotated, "")
	badOrigin.Header.Set("Origin", "https://evil.example")
	h.ServeHTTP(rec, badOrigin)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("logout evil origin %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/auth/logout", cookie, rotated, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("logout %d %s", rec.Code, rec.Body.String())
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("cookie not cleared")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("session after logout %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz %d", rec.Code)
	}
}

func TestSuccessfulLoginClearsLockout(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	for i := 0; i < 4; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, loginRequest("wrong-password-x"))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("fail %d %d", i, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loginRequest(ownerPassword))
	if rec.Code != http.StatusOK {
		t.Fatalf("success after failures %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, loginRequest("wrong-password-x"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("after clear %d", rec.Code)
	}
}

func TestLoginStorageErrorDoesNotIncrementLockout(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	if _, err := srv.db.Exec(`DROP TABLE owners`); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, loginRequest(ownerPassword))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeDependencyUnavailable {
		t.Fatalf("code = %q", body.Error.Code)
	}
	var n int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM login_attempts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("lockout incremented on storage error: %d", n)
	}
}

func TestFailedLoginAttemptPersistenceFailureFailsClosed(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	if _, err := srv.db.Exec(`DROP TABLE login_attempts`); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, loginRequest("wrong-password-x"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeDependencyUnavailable {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestLoginRateLimitsIPWideUsernameSpray(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	for i := 0; i < 20; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(
			`{"username":"unknown`+strconv.Itoa(i)+`","password":"wrong-password-x"}`,
		))
		req.Header.Set("Origin", "http://127.0.0.1:8790")
		req.RemoteAddr = "10.0.0.8:1234"
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("spray %d status = %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	req := loginRequest(ownerPassword)
	req.RemoteAddr = "10.0.0.8:9999"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("spray lockout status = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
}

func TestLoginInvalidUsernameIsGenericAndNotAuditedRaw(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	raw := strings.Repeat("a", 65)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"`+raw+`","password":"wrong-password-x"}`))
	req.Header.Set("Origin", "http://127.0.0.1:8790")
	req.RemoteAddr = "127.0.0.1:12345"
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), raw) || strings.Contains(rec.Body.String(), "\x00") {
		t.Fatalf("raw username leaked: %s", rec.Body.String())
	}
	var actor, target, metadata string
	if err := srv.db.QueryRow(`SELECT actor, target, metadata FROM audit_events WHERE action = 'owner.login'`).Scan(&actor, &target, &metadata); err != nil {
		t.Fatal(err)
	}
	if actor != "invalid_username" || target != "invalid_username" {
		t.Fatalf("actor/target = %q %q", actor, target)
	}
	if strings.Contains(metadata, raw) || strings.Contains(actor, "\x00") {
		t.Fatalf("raw username stored: %s", metadata)
	}
	var n int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM login_attempts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("invalid username spray attempt not persisted: %d", n)
	}
}

func TestSessionToolLinksDefaultEmptyObject(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)

	var before int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&before); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store, max-age=0" {
		t.Fatal("session cache")
	}
	if rec.Header().Get("Pragma") != "no-cache" {
		t.Fatal("session pragma")
	}
	var sess map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	links, ok := sess["tool_links"].(map[string]any)
	if !ok {
		t.Fatalf("tool_links = %#v", sess["tool_links"])
	}
	if len(links) != 0 {
		t.Fatalf("tool_links = %#v", links)
	}

	var after int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("GET session wrote audit: %d -> %d", before, after)
	}
}

func TestSessionToolLinksOmitsEmptyKeys(t *testing.T) {
	srv, _ := testServer(t, nil)
	srv.cfg.PgAdminURL = "https://pgadmin.example.com/browser"
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session %d %s", rec.Code, rec.Body.String())
	}
	var sess map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	links, _ := sess["tool_links"].(map[string]any)
	if links["pgadmin"] != "https://pgadmin.example.com/browser" {
		t.Fatalf("pgadmin = %#v", links["pgadmin"])
	}
	if _, ok := links["redisinsight"]; ok {
		t.Fatalf("empty redisinsight present: %#v", links)
	}
	if links["pgadmin"] == "" || links["pgadmin"] == nil {
		t.Fatal("pgadmin empty or null")
	}
}

func TestSessionToolLinksBothHrefs(t *testing.T) {
	srv, _ := testServer(t, nil)
	srv.cfg.PgAdminURL = "https://pgadmin.example.com"
	srv.cfg.RedisInsightURL = "https://redis-insight.example.com"
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session %d %s", rec.Code, rec.Body.String())
	}
	var sess map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	links, _ := sess["tool_links"].(map[string]any)
	if links["pgadmin"] != "https://pgadmin.example.com" {
		t.Fatalf("pgadmin = %#v", links["pgadmin"])
	}
	if links["redisinsight"] != "https://redis-insight.example.com" {
		t.Fatalf("redisinsight = %#v", links["redisinsight"])
	}
}

func TestSessionToolLinksRedisInsightAlone(t *testing.T) {
	srv, _ := testServer(t, nil)
	srv.cfg.RedisInsightURL = "https://redis-insight.example.com"
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session %d %s", rec.Code, rec.Body.String())
	}
	var sess map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	links, _ := sess["tool_links"].(map[string]any)
	if links["redisinsight"] != "https://redis-insight.example.com" {
		t.Fatalf("redisinsight = %#v", links["redisinsight"])
	}
	if _, ok := links["pgadmin"]; ok {
		t.Fatalf("empty pgadmin present: %#v", links)
	}
}

func seedLoginFailures(t *testing.T, srv *Server, username, ip string, n int, now time.Time) {
	t.Helper()
	store := auth.AttemptStore{DB: srv.db}
	for i := 0; i < n; i++ {
		if err := store.Record(username, ip, false, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoginIgnoresSpoofedCFConnectingIPFromNonLoopback(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	now := time.Now().UTC()
	seedLoginFailures(t, srv, "admin", "10.0.0.8", 5, now)

	rec := httptest.NewRecorder()
	req := loginRequest(ownerPassword)
	req.RemoteAddr = "10.0.0.8:9999"
	req.Header.Set("CF-Connecting-IP", "203.0.113.10")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("spoofed header still used remote IP; status = %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = loginRequest(ownerPassword)
	req.RemoteAddr = "10.0.0.9:9999"
	req.Header.Set("CF-Connecting-IP", "203.0.113.10")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("spoofed CF-Connecting-IP locked a different RemoteAddr: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginLoopbackUsesValidCFConnectingIP(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	now := time.Now().UTC()
	seedLoginFailures(t, srv, "admin", "203.0.113.10", 5, now)

	rec := httptest.NewRecorder()
	req := loginRequest(ownerPassword)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("CF-Connecting-IP", "203.0.113.10")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("loopback+header did not use header IP: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = loginRequest(ownerPassword)
	req.RemoteAddr = "127.0.0.1:12345"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback without header used header IP: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginLoopbackSpraySkippedWithoutOrInvalidCFConnectingIP(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	now := time.Now().UTC()
	for i := 0; i < 20; i++ {
		seedLoginFailures(t, srv, "unknown"+strconv.Itoa(i), "127.0.0.1", 1, now.Add(time.Duration(i)*time.Second))
	}

	rec := httptest.NewRecorder()
	req := loginRequest(ownerPassword)
	req.RemoteAddr = "127.0.0.1:12345"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback spray without header = %d %s", rec.Code, rec.Body.String())
	}

	for _, invalid := range []string{"", "not-an-ip", "203.0.113.10,198.51.100.1", "203.0.113.10:443"} {
		rec = httptest.NewRecorder()
		req = loginRequest("wrong-password-x")
		req.RemoteAddr = "127.0.0.1:12345"
		if invalid != "" {
			req.Header.Set("CF-Connecting-IP", invalid)
		}
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("invalid header %q spray-locked loopback: %d %s", invalid, rec.Code, rec.Body.String())
		}
	}
}

func TestLoginLoopbackSprayUsesValidCFConnectingIP(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	now := time.Now().UTC()
	for i := 0; i < 20; i++ {
		seedLoginFailures(t, srv, "unknown"+strconv.Itoa(i), "203.0.113.10", 1, now.Add(time.Duration(i)*time.Second))
	}

	rec := httptest.NewRecorder()
	req := loginRequest(ownerPassword)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("CF-Connecting-IP", "203.0.113.10")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("loopback+header spray = %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginXForwardedForNeverAffectsLockout(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	now := time.Now().UTC()
	for i := 0; i < 20; i++ {
		seedLoginFailures(t, srv, "unknown"+strconv.Itoa(i), "203.0.113.10", 1, now.Add(time.Duration(i)*time.Second))
	}

	rec := httptest.NewRecorder()
	req := loginRequest(ownerPassword)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("X-Real-IP", "203.0.113.10")
	req.Header.Set("True-Client-IP", "203.0.113.10")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("forwarded headers affected lockout: %d %s", rec.Code, rec.Body.String())
	}

	seedLoginFailures(t, srv, "admin", "127.0.0.1", 5, time.Now().UTC().Add(time.Minute))
	rec = httptest.NewRecorder()
	req = loginRequest(ownerPassword)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.20")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("X-Forwarded-For bypassed username lockout: %d %s", rec.Code, rec.Body.String())
	}
}

func TestConcurrentLoginReservesFailureBeforeHash(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	const extra = 3
	n := 5 + extra
	codes := make([]int, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := loginRequest("wrong-password-x")
			req.RemoteAddr = "10.0.0.8:1234"
			h.ServeHTTP(rec, req)
			codes[i] = rec.Code
		}(i)
	}
	wg.Wait()
	unauthorized, limited := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusUnauthorized:
			unauthorized++
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Fatalf("status %d codes=%v", code, codes)
		}
	}
	if unauthorized != 5 || limited != extra {
		t.Fatalf("unauthorized=%d limited=%d codes=%v", unauthorized, limited, codes)
	}
}
