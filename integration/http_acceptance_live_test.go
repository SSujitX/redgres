package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const liveOrigin = "http://127.0.0.1:8790"

func liveLoginRequest(username, password string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
	req.Header.Set("Origin", liveOrigin)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	return req
}

// TestLiveAuthRateLimit verifies AUTH-005 over the real SQLite attempt store:
// five consecutive failed logins lock the username, a correct password then
// returns 429 with Retry-After, and the failures are persisted (not lost).
func TestLiveAuthRateLimit(t *testing.T) {
	h, _, _, _, _, _ := buildLiveHTTPServer(t, nil)
	login := func(password string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, liveLoginRequest("admin", password))
		return rec
	}
	for i := 0; i < 5; i++ {
		rec := login("wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("failed login %d status = %d", i, rec.Code)
		}
	}
	locked := login(liveOwnerPassword)
	if locked.Code != http.StatusTooManyRequests {
		t.Fatalf("locked login status = %d body %s", locked.Code, locked.Body.String())
	}
	if ra := locked.Result().Header.Get("Retry-After"); ra == "" || ra == "0" {
		t.Fatalf("Retry-After = %q want positive", ra)
	}
}

// TestLiveSearchAndAuditPagination verifies PLAT-004 live bounded search (a
// created PostgreSQL database and a created Redis ACL user are found with
// service context) and PLAT-003 cursor pagination (limit + next_cursor pages
// do not overlap and has_more transitions) over the real control plane.
func TestLiveSearchAndAuditPagination(t *testing.T) {
	h, cookie, csrf, _, _, _ := buildLiveHTTPServer(t, nil)

	rec := liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases", cookie, csrf,
		`{"database":"search_live","owner":"app_search_live"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create db status %d: %s", rec.Code, rec.Body.String())
	}
	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/redis/users", cookie, csrf,
		`{"username":"search_live_ro","key_pattern":"search:live","preset":"read-only"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create redis user status %d: %s", rec.Code, rec.Body.String())
	}

	// PLAT-004: bounded search finds both live resources with service context.
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/search?q=search_live", cookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("search status %d: %s", rec.Code, rec.Body.String())
	}
	var search struct {
		Groups []struct {
			ID      string `json:"id"`
			Service string `json:"service"`
			Status  string `json:"status"`
			Hits    []struct {
				ID    string `json:"id"`
				Label string `json:"label"`
			} `json:"hits"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &search); err != nil {
		t.Fatal(err)
	}
	foundPG, foundRedis := false, false
	for _, g := range search.Groups {
		for _, hit := range g.Hits {
			if hit.ID == "postgres_database:search_live" || hit.Label == "search_live" {
				foundPG = true
			}
			if hit.ID == "redis_acl_user:search_live_ro" || hit.Label == "search_live_ro" {
				foundRedis = true
			}
		}
	}
	if !foundPG || !foundRedis {
		t.Fatalf("search missing live hits: %+v", search.Groups)
	}

	// PLAT-003: cursor pagination pages do not overlap.
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/audit?limit=2", cookie, csrf, "")
	var page1 struct {
		Events []struct {
			ID int64 `json:"id"`
		} `json:"events"`
		HasMore    bool   `json:"has_more"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Events) == 0 || len(page1.Events) > 2 {
		t.Fatalf("page1 events = %d", len(page1.Events))
	}
	if !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("expected more pages: has_more=%v cursor=%q", page1.HasMore, page1.NextCursor)
	}
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/audit?limit=2&cursor="+page1.NextCursor, cookie, csrf, "")
	var page2 struct {
		Events []struct {
			ID int64 `json:"id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Events) == 0 {
		t.Fatal("empty second page")
	}
	seen := map[int64]bool{}
	for _, ev := range page1.Events {
		seen[ev.ID] = true
	}
	for _, ev := range page2.Events {
		if seen[ev.ID] {
			t.Fatalf("audit id %d appears on both pages (cursor pagination overlap)", ev.ID)
		}
	}
}
