package operations

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTransitionRejectsUnsafeResultCanaries(t *testing.T) {
	t.Run("postgres_url", func(t *testing.T) {
		db, store := newTestDB(t)
		id := runningDuplicate(t, store)
		err := store.Transition(context.Background(), id, Transition{
			From: StatusRunning,
			To:   StatusSucceeded,
			Result: &DuplicateResult{
				Database: "postgres://owner:canary-secret@127.0.0.1/db",
				Owner:    "app_project_a_copy",
				Source:   "project_a",
			},
		})
		if !errors.Is(err, ErrUnsafeResult) {
			t.Fatalf("got %v, want ErrUnsafeResult", err)
		}
		assertUnchangedRunning(t, store, id)
		assertNoCanaryStored(t, db, id)
	})
	t.Run("canary_secret_value", func(t *testing.T) {
		db, store := newTestDB(t)
		id := runningDuplicate(t, store)
		err := store.Transition(context.Background(), id, Transition{
			From: StatusRunning,
			To:   StatusSucceeded,
			Result: &DuplicateResult{
				Database: "project_a_copy",
				Owner:    "canary-secret",
				Source:   "project_a",
			},
		})
		if !errors.Is(err, ErrUnsafeResult) {
			t.Fatalf("got %v, want ErrUnsafeResult", err)
		}
		assertUnchangedRunning(t, store, id)
		assertNoCanaryStored(t, db, id)
	})
	t.Run("password_key", func(t *testing.T) {
		err := checkResultJSON([]byte(`{"database":"project_a_copy","owner":"app_project_a_copy","source":"project_a","password":"canary-secret"}`))
		if !errors.Is(err, ErrUnsafeResult) {
			t.Fatalf("got %v, want ErrUnsafeResult", err)
		}
	})
	t.Run("extra_key", func(t *testing.T) {
		err := checkResultJSON([]byte(`{"database":"project_a_copy","owner":"app_project_a_copy","source":"project_a","note":"extra"}`))
		if !errors.Is(err, ErrUnsafeResult) {
			t.Fatalf("got %v, want ErrUnsafeResult", err)
		}
	})
}

func TestTransitionRejectsUnsafeErrorCanaries(t *testing.T) {
	cases := []OperationError{
		{Code: "failed", Message: "Duplicate did not finish.", Fields: map[string]string{"password": "canary-secret"}},
		{Code: "failed", Message: "postgres://owner:canary-secret@127.0.0.1/db"},
		{Code: "failed", Message: "canary-secret"},
	}
	for _, opErr := range cases {
		t.Run(opErr.Message+opErr.Fields["password"], func(t *testing.T) {
			db, store := newTestDB(t)
			id := runningDuplicate(t, store)
			err := store.Transition(context.Background(), id, Transition{
				From:  StatusRunning,
				To:    StatusFailed,
				Error: &opErr,
			})
			if !errors.Is(err, ErrUnsafeError) {
				t.Fatalf("got %v, want ErrUnsafeError", err)
			}
			assertUnchangedRunning(t, store, id)
			assertNoCanaryStored(t, db, id)
		})
	}
}

func TestReconcileProbeErrorDoesNotStoreDump(t *testing.T) {
	db, store := newTestDB(t)
	id := runningDuplicate(t, store)
	now := time.Date(2026, 8, 26, 6, 0, 0, 0, time.UTC)
	probe := &fakeProbe{err: errors.New("pq: password=canary-secret postgres://owner:canary-secret@127.0.0.1/db")}
	if err := store.Reconcile(context.Background(), probe, nil, now); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusIndeterminate {
		t.Fatalf("status = %q", got.Status)
	}
	if got.Error != nil && (strings.Contains(got.Error.Message, "canary-secret") || strings.Contains(got.Error.Message, "postgres://")) {
		t.Fatalf("probe dump stored: %#v", got.Error)
	}
	assertNoCanaryStored(t, db, id)
}

func runningDuplicate(t *testing.T, store Store) string {
	t.Helper()
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), op.ID, Transition{From: StatusQueued, To: StatusRunning, Phase: PhaseCloning}); err != nil {
		t.Fatal(err)
	}
	return op.ID
}

func assertUnchangedRunning(t *testing.T, store Store, id string) {
	t.Helper()
	got, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusRunning || got.Result != nil || got.Error != nil {
		t.Fatalf("row changed: %#v", got)
	}
}

func assertNoCanaryStored(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	var resultJSON, errorJSON sql.NullString
	if err := db.QueryRow(`SELECT result_json, error_json FROM operations WHERE id = ?`, id).Scan(&resultJSON, &errorJSON); err != nil {
		t.Fatal(err)
	}
	blob := strings.ToLower(resultJSON.String + "\n" + errorJSON.String)
	for _, needle := range []string{"canary-secret", "postgres://", "postgresql://", `"password"`} {
		if strings.Contains(blob, needle) {
			t.Fatalf("secret material stored: %q %q", resultJSON.String, errorJSON.String)
		}
	}
}
