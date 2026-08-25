package operations

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/migrations"
)

func TestGetRejectsInvalidID(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Get(context.Background(), "not-an-id")
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("got %v, want ErrInvalidID", err)
	}
}

func TestGetMissingID(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Get(context.Background(), mustID(t))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestInsertQueuedRoundTrip(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != op.ID || got.Action != ActionDuplicate || got.Status != StatusQueued {
		t.Fatalf("got %#v", got)
	}
	if got.Actor != op.Actor || got.Target != op.Target || got.AcceptedRequestID != op.AcceptedRequestID {
		t.Fatalf("identity mismatch: %#v", got)
	}
	if got.Phase != PhaseAccepted {
		t.Fatalf("phase = %q", got.Phase)
	}
	if got.Result != nil {
		t.Fatalf("queued result = %#v", got.Result)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("times not stored: %#v", got)
	}
	if got.StartedAt != nil || got.FinishedAt != nil {
		t.Fatalf("queued must not have start/finish: %#v", got)
	}
}

func TestInsertQueuedRejectsOtherAction(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	op.Action = "postgres.database.create"
	err := store.InsertQueued(context.Background(), op, locks)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
	_, err = store.Get(context.Background(), op.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("rejected insert persisted: %v", err)
	}
}

func TestInsertQueuedRejectsInvalidID(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	op.ID = "ABCDEF0123456789ABCDEF0123456789"
	err := store.InsertQueued(context.Background(), op, locks)
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("got %v, want ErrInvalidID", err)
	}
}

func TestInsertQueuedRejectsUnknownLockKind(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	locks[0].Kind = "postgres.table"
	err := store.InsertQueued(context.Background(), op, locks)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
}

func newTestStore(t *testing.T) Store {
	t.Helper()
	_, store := newTestDB(t)
	return store
}

func newTestDB(t *testing.T) (*sql.DB, Store) {
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
	return db, NewStore(db)
}

func queuedDuplicate(t *testing.T, source, database, owner string) (Operation, []ResourceLock) {
	t.Helper()
	now := time.Date(2026, 8, 26, 5, 0, 0, 0, time.UTC)
	op := Operation{
		ID:                mustID(t),
		Action:            ActionDuplicate,
		Status:            StatusQueued,
		Phase:             PhaseAccepted,
		Actor:             "admin",
		Target:            database,
		AcceptedRequestID: mustID(t),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	locks := []ResourceLock{
		{Kind: ResourceDatabase, Name: source},
		{Kind: ResourceDatabase, Name: database},
		{Kind: ResourceRole, Name: owner},
	}
	return op, locks
}

func mustID(t *testing.T) string {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 32 {
		t.Fatalf("id length = %d", len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Fatalf("id is not hex: %v", err)
	}
	return id
}
