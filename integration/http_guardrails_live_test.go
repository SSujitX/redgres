package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
)

// TestLiveDuplicateInFlightConflict verifies the PG-010 operation lock over
// HTTP: while a duplicate is queued (no worker in this harness), a second
// duplicate targeting the same database is rejected 409 operation_in_progress.
func TestLiveDuplicateInFlightConflict(t *testing.T) {
	h, cookie, csrf, _, _, _ := buildLiveHTTPServer(t, nil)

	rec := liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases", cookie, csrf,
		`{"database":"conflict_live","owner":"app_conflict_live"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", rec.Code, rec.Body.String())
	}

	dupBody := `{"database":"conflict_live_copy","owner":"app_conflict_live_copy"}`
	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases/conflict_live/duplicate", cookie, csrf, dupBody)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("first duplicate status %d: %s", rec.Code, rec.Body.String())
	}

	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases/conflict_live/duplicate", cookie, csrf, dupBody)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second duplicate status %d: %s", rec.Code, rec.Body.String())
	}
	var errBody struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if errBody.Error.Code != "operation_in_progress" {
		t.Fatalf("conflict code = %q", errBody.Error.Code)
	}
}

// TestLiveSessionExpiry verifies AUTH-003 session expiry over the real
// session store: with a short idle TTL, an authenticated request succeeds
// immediately and is rejected 401 after the session expires.
func TestLiveSessionExpiry(t *testing.T) {
	h, _, _, _, _, _ := buildLiveHTTPServer(t, func(c *config.Config) {
		c.SessionTTL = 500 * time.Millisecond
		c.AbsoluteSessionTTL = 10 * time.Second
	})
	cookie, csrf := liveLogin(t, h)
	rec := liveAuthed(t, h, http.MethodGet, "/api/v1/status", cookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("immediate status %d", rec.Code)
	}
	time.Sleep(1200 * time.Millisecond)
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/status", cookie, csrf, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired status %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "status") && strings.Contains(rec.Body.String(), "components") {
		t.Fatal("expired session leaked component data")
	}
}
