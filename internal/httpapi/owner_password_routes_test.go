package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func changePasswordJSON(current, next string) string {
	return `{"current_password":"` + current + `","new_password":"` + next + `"}`
}

func TestChangePasswordSuccessAndRotates(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	cookie, csrf := login(t, srv.Handler())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, authed(http.MethodPost, "/api/v1/auth/password", cookie, csrf, changePasswordJSON(ownerPassword, "new-owner-secret-17")))
	if rec.Code != http.StatusOK {
		t.Fatalf("change status = %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store, max-age=0" {
		t.Fatalf("Cache-Control = %q, want no-store, max-age=0", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != true {
		t.Fatalf("body = %#v", body)
	}

	old := httptest.NewRecorder()
	srv.Handler().ServeHTTP(old, loginRequest(ownerPassword))
	if old.Code != http.StatusUnauthorized {
		t.Fatalf("old password login = %d, want 401", old.Code)
	}
	fresh := httptest.NewRecorder()
	srv.Handler().ServeHTTP(fresh, loginRequest("new-owner-secret-17"))
	if fresh.Code != http.StatusOK {
		t.Fatalf("new password login = %d %s, want 200", fresh.Code, fresh.Body.String())
	}
}

func TestChangePasswordRejectsWrongCurrent(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	cookie, csrf := login(t, srv.Handler())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, authed(http.MethodPost, "/api/v1/auth/password", cookie, csrf, changePasswordJSON("wrong-current-pass", "new-owner-secret-17")))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeReauthRequired {
		t.Fatalf("code = %q, want %q", body.Error.Code, CodeReauthRequired)
	}

	var failures int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action = ? AND outcome = ?`, "owner.password_change", "failure").Scan(&failures); err != nil {
		t.Fatal(err)
	}
	if failures != 1 {
		t.Fatalf("failure audit rows = %d, want 1", failures)
	}
}

func TestChangePasswordRejectsSameAsCurrent(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	cookie, csrf := login(t, srv.Handler())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, authed(http.MethodPost, "/api/v1/auth/password", cookie, csrf, changePasswordJSON(ownerPassword, ownerPassword)))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Fields["new_password"] != "same_as_current" {
		t.Fatalf("fields = %#v, want new_password=same_as_current", body.Error.Fields)
	}
}

func TestChangePasswordRejectsWeak(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	cookie, csrf := login(t, srv.Handler())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, authed(http.MethodPost, "/api/v1/auth/password", cookie, csrf, changePasswordJSON(ownerPassword, "short")))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeValidationError {
		t.Fatalf("code = %q, want %q", body.Error.Code, CodeValidationError)
	}
}

func TestChangePasswordRequiresCSRF(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	cookie, _ := login(t, srv.Handler())

	req := authed(http.MethodPost, "/api/v1/auth/password", cookie, "", changePasswordJSON(ownerPassword, "new-owner-secret-17"))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestChangePasswordRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(changePasswordJSON(ownerPassword, "new-owner-secret-17")))
	req.Header.Set("Origin", "http://127.0.0.1:8790")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestChangePasswordInvalidatesOldSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	cookie, csrf := login(t, srv.Handler())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, authed(http.MethodPost, "/api/v1/auth/password", cookie, csrf, changePasswordJSON(ownerPassword, "new-owner-secret-17")))
	if rec.Code != http.StatusOK {
		t.Fatalf("change status = %d %s", rec.Code, rec.Body.String())
	}

	old := httptest.NewRecorder()
	srv.Handler().ServeHTTP(old, authed(http.MethodGet, "/api/v1/session", cookie, "", ""))
	if old.Code != http.StatusUnauthorized {
		t.Fatalf("old session status = %d, want 401", old.Code)
	}
}
