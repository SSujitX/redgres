package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/audit"
)

func auditGet(t *testing.T, srv *Server, query, cookie string) (*httptest.ResponseRecorder, auditEventsBody) {
	t.Helper()
	path := "/api/v1/audit"
	if query != "" {
		path += "?" + query
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, authed(http.MethodGet, path, cookie, "", ""))
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var body auditEventsBody
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s: %v", rec.Body.String(), err)
		}
		if body.HasMore != (body.NextCursor != "") {
			t.Fatalf("has_more = %t but next_cursor = %q", body.HasMore, body.NextCursor)
		}
		if !requestIDOK(body.RequestID) {
			t.Fatalf("request_id = %q", body.RequestID)
		}
		if rec.Header().Get("X-Request-ID") != body.RequestID {
			t.Fatal("header/body request id mismatch")
		}
	}
	return rec, body
}

func auditSession(t *testing.T) (*Server, string) {
	t.Helper()
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	cookie, _ := login(t, srv.Handler())
	return srv, cookie
}

func TestAuditListShowsLoginSuccess(t *testing.T) {
	srv, cookie := auditSession(t)

	rec, body := auditGet(t, srv, "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var found bool
	for _, event := range body.Events {
		if event.Action == "owner.login" && event.Outcome == "success" {
			found = true
			if event.Actor != "admin" {
				t.Fatalf("actor = %q", event.Actor)
			}
			if !requestIDOK(event.RequestID) {
				t.Fatalf("event request_id = %q", event.RequestID)
			}
			if event.ClientIP != "127.0.0.1" {
				t.Fatalf("client_ip = %q", event.ClientIP)
			}
			if event.CreatedAt == "" {
				t.Fatal("created_at is empty")
			}
		}
	}
	if !found {
		t.Fatalf("no successful login event in %s", rec.Body.String())
	}
}

func TestAuditListShowsFailureEvents(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, loginRequest("wrong-password-value"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d", rec.Code)
	}
	cookie, _ := login(t, srv.Handler())

	_, body := auditGet(t, srv, "", cookie)
	outcomes := map[string]bool{}
	for _, event := range body.Events {
		outcomes[event.Outcome] = true
	}
	if !outcomes["failure"] || !outcomes["success"] {
		t.Fatalf("outcomes = %v", outcomes)
	}
}

func TestAuditListPagesStably(t *testing.T) {
	srv, cookie := auditSession(t)
	store := audit.Store{DB: srv.db}
	if _, err := store.DB.Exec(`DELETE FROM audit_events`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := store.Record("admin", "owner.login", "admin", "success", "aabbccddeeff00112233445566778899", "127.0.0.1", nil); err != nil {
			t.Fatal(err)
		}
	}

	first, firstBody := auditGet(t, srv, "limit=2", cookie)
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d", first.Code)
	}
	if len(firstBody.Events) != 2 || !firstBody.HasMore {
		t.Fatalf("first page = %+v", firstBody)
	}
	if firstBody.Limit != 2 {
		t.Fatalf("limit echo = %d", firstBody.Limit)
	}

	// A write between the two reads must not shift or duplicate page two.
	if err := store.Record("admin", "owner.logout", "admin", "success", "aabbccddeeff00112233445566778899", "127.0.0.1", nil); err != nil {
		t.Fatal(err)
	}

	seen := map[int64]bool{}
	for _, event := range firstBody.Events {
		seen[event.ID] = true
	}
	cursor := firstBody.NextCursor
	for page := 1; page < 3; page++ {
		_, body := auditGet(t, srv, "limit=2&cursor="+cursor, cookie)
		wantLen := 2
		if page == 2 {
			wantLen = 1
		}
		if len(body.Events) != wantLen {
			t.Fatalf("page %d length = %d, want %d", page, len(body.Events), wantLen)
		}
		for _, event := range body.Events {
			if seen[event.ID] {
				t.Fatalf("id %d repeated", event.ID)
			}
			seen[event.ID] = true
		}
		if page == 2 {
			if body.HasMore || body.NextCursor != "" {
				t.Fatalf("last page = %+v", body)
			}
			break
		}
		cursor = body.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("saw %d distinct ids, want 5", len(seen))
	}
}

func TestAuditListClampsLimit(t *testing.T) {
	srv, cookie := auditSession(t)
	for _, query := range []string{"limit=0", "limit=-3", "limit=501"} {
		rec, body := auditGet(t, srv, query, cookie)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", query, rec.Code)
		}
		if body.Limit != audit.DefaultListLimit {
			t.Fatalf("%s limit echo = %d", query, body.Limit)
		}
	}
}

func TestAuditListEmptyIsEmptyArray(t *testing.T) {
	srv, cookie := auditSession(t)
	if _, err := srv.db.Exec(`DELETE FROM audit_events`); err != nil {
		t.Fatal(err)
	}
	rec, body := auditGet(t, srv, "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(body.Events) != 0 || body.HasMore {
		t.Fatalf("body = %+v", body)
	}
	if !strings.Contains(rec.Body.String(), `"events":[]`) {
		t.Fatalf("events not an empty array: %s", rec.Body.String())
	}
}

func TestAuditListCursorBelowOldestIsEmptyNotError(t *testing.T) {
	srv, cookie := auditSession(t)
	var lowest int64
	if err := srv.db.QueryRow(`SELECT min(id) FROM audit_events`).Scan(&lowest); err != nil {
		t.Fatal(err)
	}
	rec, body := auditGet(t, srv, "cursor="+encodeAuditCursor(lowest), cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if len(body.Events) != 0 || body.HasMore {
		t.Fatalf("body = %+v", body)
	}
}

func TestAuditListRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)

	for _, cookie := range []string{"", "not-a-real-session-token"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, authed(http.MethodGet, "/api/v1/audit", cookie, "", ""))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("cookie %q status = %d", cookie, rec.Code)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
		}
		if strings.Contains(rec.Body.String(), `"events"`) {
			t.Fatalf("unauthorized body exposed events: %s", rec.Body.String())
		}
	}
}

func TestAuditListRejectsMalformedCursor(t *testing.T) {
	srv, cookie := auditSession(t)
	cases := map[string]string{
		"not base64":        "!!!!",
		"padded base64":     base64.StdEncoding.EncodeToString([]byte("a1:5")),
		"wrong prefix":      base64.RawURLEncoding.EncodeToString([]byte("b9:5")),
		"no prefix":         base64.RawURLEncoding.EncodeToString([]byte("5")),
		"non numeric":       base64.RawURLEncoding.EncodeToString([]byte("a1:abc")),
		"zero":              base64.RawURLEncoding.EncodeToString([]byte("a1:0")),
		"negative":          base64.RawURLEncoding.EncodeToString([]byte("a1:-4")),
		"overflow":          base64.RawURLEncoding.EncodeToString([]byte("a1:999999999999999999999999999999")),
		"leading plus":      base64.RawURLEncoding.EncodeToString([]byte("a1:+5")),
		"leading zeros":     base64.RawURLEncoding.EncodeToString([]byte("a1:007")),
		"trailing space":    base64.RawURLEncoding.EncodeToString([]byte("a1:5 ")),
		"over length input": strings.Repeat("QQ", 40),
	}
	for name, cursor := range cases {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, authed(http.MethodGet, "/api/v1/audit?cursor="+cursor, cookie, "", ""))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d %s", name, rec.Code, rec.Body.String())
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if body.Error.Code != CodeValidationError {
			t.Fatalf("%s: code = %q", name, body.Error.Code)
		}
		if body.Error.Fields["cursor"] != "invalid" {
			t.Fatalf("%s: fields = %v", name, body.Error.Fields)
		}
		if strings.Contains(rec.Body.String(), cursor) {
			t.Fatalf("%s: response echoed the submitted cursor: %s", name, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: Cache-Control = %q", name, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestAuditListRejectsMalformedLimit(t *testing.T) {
	srv, cookie := auditSession(t)
	for _, limit := range []string{"abc", "1.5", "5x", "%204"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, authed(http.MethodGet, "/api/v1/audit?limit="+limit, cookie, "", ""))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("limit %q status = %d", limit, rec.Code)
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != CodeValidationError || body.Error.Fields["limit"] != "invalid" {
			t.Fatalf("limit %q body = %+v", limit, body.Error)
		}
	}
}

func TestAuditListRejectsMutationMethods(t *testing.T) {
	srv, cookie := auditSession(t)
	for _, method := range []string{http.MethodPost, http.MethodDelete, http.MethodPut} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, authed(method, "/api/v1/audit", cookie, "", ""))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d", method, rec.Code)
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

// The audit table is dropped rather than the connection closed so requireSession
// still succeeds and the failure is raised by the handler's own read path.
func TestAuditListStorageFailureDoesNotLeak(t *testing.T) {
	srv, path := testServer(t, nil)
	seedOwner(t, srv)
	cookie, _ := login(t, srv.Handler())
	if _, err := srv.db.Exec(`DROP TABLE audit_events`); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, authed(http.MethodGet, "/api/v1/audit", cookie, "", ""))
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
	if body.Error.Message != storageUnavailable {
		t.Fatalf("message = %q", body.Error.Message)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, `"events"`) {
		t.Fatalf("error body carried events: %s", raw)
	}
	lower := strings.ToLower(raw)
	for _, leak := range []string{path, "sqlite", "modernc", "audit_events", "no such table"} {
		if strings.Contains(lower, strings.ToLower(leak)) {
			t.Fatalf("error leaked %q: %s", leak, raw)
		}
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

// Metadata is never selected on the read path, so raw secret material written
// directly to the column, bypassing the write-time redactor, cannot surface.
func TestAuditListNeverReturnsMetadata(t *testing.T) {
	srv, cookie := auditSession(t)
	_, err := srv.db.Exec(
		`INSERT INTO audit_events (actor, action, target, outcome, request_id, client_ip, metadata, created_at)
		 VALUES ('admin', 'owner.login', 'admin', 'success', 'aabbccddeeff00112233445566778899', '127.0.0.1', ?, '2026-08-25T04:11:05Z')`,
		`{"admin_url":"postgresql://canary:secret@127.0.0.1/db","password":"canary-password","csrf_token":"canary-csrf","session_token":"canary-token"}`,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Prove the canary really is stored and detectable, so the assertions below
	// cannot pass just because the row is missing.
	var stored string
	if err := srv.db.QueryRow(`SELECT metadata FROM audit_events ORDER BY id DESC LIMIT 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !audit.ContainsSecret(stored) || !strings.Contains(stored, "canary") {
		t.Fatalf("canary row was not stored as written: %s", stored)
	}

	rec, body := auditGet(t, srv, "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(body.Events) == 0 {
		t.Fatal("no events returned")
	}
	var canarySeen bool
	for _, event := range body.Events {
		if event.CreatedAt == "2026-08-25T04:11:05Z" {
			canarySeen = true
		}
	}
	if !canarySeen {
		t.Fatalf("the canary event was not in the page: %s", rec.Body.String())
	}
	raw := rec.Body.String()
	if audit.ContainsSecret(raw) {
		t.Fatalf("secret material in response: %s", raw)
	}
	for _, leak := range []string{"canary", "secret", "csrf_token", "postgresql://", "metadata"} {
		if strings.Contains(strings.ToLower(raw), leak) {
			t.Fatalf("response contained %q: %s", leak, raw)
		}
	}
}

func TestAuditCapabilityGate(t *testing.T) {
	if !hasCapability("audit.read") {
		t.Fatal("audit.read is not a default capability")
	}
	if hasCapability("audit.export") {
		t.Fatal("audit.export unexpectedly granted")
	}

	srv, _ := testServer(t, nil)
	reached := false
	handler := srv.requireCapability("audit.export")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
	if reached {
		t.Fatal("handler ran without the capability")
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeForbidden {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestAuditCursorRoundTrip(t *testing.T) {
	for _, id := range []int64{1, 42, 9007199254740991} {
		got, ok := decodeAuditCursor(encodeAuditCursor(id))
		if !ok || got != id {
			t.Fatalf("round trip of %d gave %d, ok = %t", id, got, ok)
		}
	}
	if got, ok := decodeAuditCursor(""); !ok || got != 0 {
		t.Fatalf("empty cursor gave %d, ok = %t", got, ok)
	}
	if strings.ContainsAny(encodeAuditCursor(1234), "=+/") {
		t.Fatalf("cursor is not URL safe: %q", encodeAuditCursor(1234))
	}
}
