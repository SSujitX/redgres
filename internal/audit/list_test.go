package audit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/migrations"
)

func listTestStore(t *testing.T) Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "redgres.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return Store{DB: db}
}

// insertAt writes a row directly so a test can control created_at and the
// nullable columns without going through Record.
func insertAt(t *testing.T, store Store, action, createdAt string, actor, target, clientIP any) int64 {
	t.Helper()
	res, err := store.DB.Exec(
		`INSERT INTO audit_events (actor, action, target, outcome, request_id, client_ip, metadata, created_at)
		 VALUES (?, ?, ?, 'success', 'aabbccddeeff00112233445566778899', ?, '{}', ?)`,
		actor, action, target, clientIP, createdAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedEvents(t *testing.T, store Store, count int) []int64 {
	t.Helper()
	ids := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		if err := store.Record("admin", "owner.login", "admin", "success", "aabbccddeeff00112233445566778899", "127.0.0.1", nil); err != nil {
			t.Fatal(err)
		}
		var id int64
		if err := store.DB.QueryRow(`SELECT max(id) FROM audit_events`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestListReturnsNewestFirst(t *testing.T) {
	store := listTestStore(t)
	ids := seedEvents(t, store, 3)

	page, err := store.List(context.Background(), 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore {
		t.Fatal("has_more on a partial page")
	}
	if len(page.Events) != 3 {
		t.Fatalf("events = %d", len(page.Events))
	}
	want := []int64{ids[2], ids[1], ids[0]}
	for i, event := range page.Events {
		if event.ID != want[i] {
			t.Fatalf("events[%d].id = %d, want %d", i, event.ID, want[i])
		}
	}
}

func TestListPagesWithoutSkippingOrRepeating(t *testing.T) {
	store := listTestStore(t)
	ids := seedEvents(t, store, 5)

	var seen []int64
	var cursor int64
	for page := 0; page < 3; page++ {
		got, err := store.List(context.Background(), cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		wantLen := 2
		wantMore := true
		if page == 2 {
			wantLen = 1
			wantMore = false
		}
		if len(got.Events) != wantLen {
			t.Fatalf("page %d length = %d, want %d", page, len(got.Events), wantLen)
		}
		if got.HasMore != wantMore {
			t.Fatalf("page %d has_more = %t, want %t", page, got.HasMore, wantMore)
		}
		for _, event := range got.Events {
			seen = append(seen, event.ID)
		}
		cursor = got.Events[len(got.Events)-1].ID
	}

	want := []int64{ids[4], ids[3], ids[2], ids[1], ids[0]}
	if len(seen) != len(want) {
		t.Fatalf("saw %d ids, want %d", len(seen), len(want))
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("seen[%d] = %d, want %d", i, seen[i], want[i])
		}
	}
}

// A write between two page reads must not shift the older page. This is the
// direct proof of the PLAT-003 "cursor pagination is stable" criterion.
func TestListPagingIsStableAcrossInsert(t *testing.T) {
	store := listTestStore(t)
	ids := seedEvents(t, store, 4)

	first, err := store.List(context.Background(), 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	cursor := first.Events[len(first.Events)-1].ID

	seedEvents(t, store, 1)

	second, err := store.List(context.Background(), cursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Events) != 2 {
		t.Fatalf("second page length = %d", len(second.Events))
	}
	if second.Events[0].ID != ids[1] || second.Events[1].ID != ids[0] {
		t.Fatalf("second page = %d,%d want %d,%d", second.Events[0].ID, second.Events[1].ID, ids[1], ids[0])
	}
	for _, event := range second.Events {
		for _, seen := range first.Events {
			if event.ID == seen.ID {
				t.Fatalf("id %d repeated across pages", event.ID)
			}
		}
	}
}

// Two rows sharing a created_at string must still page deterministically, which
// is why id and not created_at is the ordering key.
func TestListOrdersTiedTimestamps(t *testing.T) {
	store := listTestStore(t)
	const tick = "2026-08-25T04:11:05.5Z"
	low := insertAt(t, store, "owner.login", tick, "admin", "admin", "127.0.0.1")
	high := insertAt(t, store, "owner.logout", tick, "admin", "admin", "127.0.0.1")

	first, err := store.List(context.Background(), 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 1 || first.Events[0].ID != high {
		t.Fatalf("first page = %+v, want id %d", first.Events, high)
	}
	if !first.HasMore {
		t.Fatal("has_more = false with a row remaining")
	}
	second, err := store.List(context.Background(), high, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Events) != 1 || second.Events[0].ID != low {
		t.Fatalf("second page = %+v, want id %d", second.Events, low)
	}
	if second.HasMore {
		t.Fatal("has_more = true on the last page")
	}
}

// created_at is stored as RFC3339Nano TEXT, whose fractional second drops
// trailing zeros, so SQLite's string ordering is not chronological. This pins
// the reason the ordering key is id.
func TestCreatedAtStringOrderIsNotChronological(t *testing.T) {
	store := listTestStore(t)
	earlier := insertAt(t, store, "earlier", "2026-08-25T04:11:05Z", "admin", "admin", "127.0.0.1")
	later := insertAt(t, store, "later", "2026-08-25T04:11:05.5Z", "admin", "admin", "127.0.0.1")

	var firstAction string
	if err := store.DB.QueryRow(`SELECT action FROM audit_events ORDER BY created_at DESC LIMIT 1`).Scan(&firstAction); err != nil {
		t.Fatal(err)
	}
	if firstAction != "earlier" {
		t.Fatalf("created_at DESC put %q first; the string-ordering hazard this guards may have changed", firstAction)
	}

	page, err := store.List(context.Background(), 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Events[0].ID != later || page.Events[1].ID != earlier {
		t.Fatalf("id ordering = %d,%d want %d,%d", page.Events[0].ID, page.Events[1].ID, later, earlier)
	}
}

func TestListEmitsEmptySliceNotNil(t *testing.T) {
	store := listTestStore(t)
	page, err := store.List(context.Background(), 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Events == nil {
		t.Fatal("events is nil")
	}
	if len(page.Events) != 0 || page.HasMore {
		t.Fatalf("page = %+v", page)
	}
}

func TestListCursorBelowOldestReturnsEmptyPage(t *testing.T) {
	store := listTestStore(t)
	ids := seedEvents(t, store, 1)

	page, err := store.List(context.Background(), ids[0], 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 0 || page.HasMore {
		t.Fatalf("page = %+v", page)
	}
}

func TestListReadsNullColumnsAsEmptyStrings(t *testing.T) {
	store := listTestStore(t)
	insertAt(t, store, "owner.login", "2026-08-25T04:11:05Z", nil, nil, nil)

	page, err := store.List(context.Background(), 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("events = %d", len(page.Events))
	}
	event := page.Events[0]
	if event.Actor != "" || event.Target != "" || event.ClientIP != "" {
		t.Fatalf("null columns = %q/%q/%q", event.Actor, event.Target, event.ClientIP)
	}
	if event.Action != "owner.login" {
		t.Fatalf("action = %q", event.Action)
	}
}

func TestClampListLimit(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, DefaultListLimit},
		{-1, DefaultListLimit},
		{MaxListLimit + 1, DefaultListLimit},
		{1, 1},
		{MaxListLimit, MaxListLimit},
	}
	for _, tc := range cases {
		if got := ClampListLimit(tc.in); got != tc.want {
			t.Fatalf("ClampListLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestListRespectsClampedLimit(t *testing.T) {
	store := listTestStore(t)
	seedEvents(t, store, 3)

	for _, limit := range []int{0, -5, MaxListLimit + 1} {
		page, err := store.List(context.Background(), 0, limit)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Events) != 3 || page.HasMore {
			t.Fatalf("limit %d gave %+v", limit, page)
		}
	}
}

// Ordering by the rowid alias must not sort, which is why this slice needs no
// new index and therefore no migration. The plan text is also asserted to be
// non-empty so an EXPLAIN returning no rows cannot pass this vacuously.
func TestListPlanUsesNoTemporaryBTree(t *testing.T) {
	store := listTestStore(t)
	for _, stmt := range []struct {
		name string
		sql  string
		args []any
		want string
	}{
		{"newest", listNewestStmt, []any{1}, "audit_events"},
		{"before", listBeforeStmt, []any{2, 1}, "INTEGER PRIMARY KEY"},
	} {
		rows, err := store.DB.Query("EXPLAIN QUERY PLAN "+stmt.sql, stmt.args...)
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for rows.Next() {
			var id, parent, notUsed int
			var detail string
			if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			plan.WriteString(detail)
			plan.WriteString("\n")
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		_ = rows.Close()
		if !strings.Contains(plan.String(), stmt.want) {
			t.Fatalf("%s plan is empty or unexpected, so the sort assertion below would be vacuous:\n%s", stmt.name, plan.String())
		}
		if strings.Contains(strings.ToUpper(plan.String()), "TEMP B-TREE") {
			t.Fatalf("%s plan sorts:\n%s", stmt.name, plan.String())
		}
	}
}

func TestListSurfacesQueryFailure(t *testing.T) {
	store := listTestStore(t)
	if _, err := store.DB.Exec(`DROP TABLE audit_events`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background(), 0, 10); err == nil {
		t.Fatal("expected an error with the table missing")
	}
}

func TestListHonorsContextCancellation(t *testing.T) {
	store := listTestStore(t)
	seedEvents(t, store, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.List(ctx, 0, 10); err == nil {
		t.Fatal("expected a cancellation error")
	}
}
