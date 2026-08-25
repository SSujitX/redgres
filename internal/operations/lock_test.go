package operations

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInsertQueuedRejectsHeldLock(t *testing.T) {
	store := newTestStore(t)
	first, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), first, locks); err != nil {
		t.Fatal(err)
	}
	second, overlap := queuedDuplicate(t, "project_a", "project_b_copy", "app_project_b_copy")
	err := store.InsertQueued(context.Background(), second, overlap)
	if !errors.Is(err, ErrLockHeld) {
		t.Fatalf("got %v, want ErrLockHeld", err)
	}
	_, err = store.Get(context.Background(), second.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("lock conflict persisted: %v", err)
	}
}

func TestTerminalTransitionReleasesLocks(t *testing.T) {
	store := newTestStore(t)
	first, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), first, locks); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 8, 26, 5, 1, 0, 0, time.UTC)
	if err := store.Transition(context.Background(), first.ID, Transition{
		From:      StatusQueued,
		To:        StatusRunning,
		Phase:     PhaseCloning,
		StartedAt: &started,
	}); err != nil {
		t.Fatal(err)
	}
	finished := started.Add(time.Minute)
	if err := store.Transition(context.Background(), first.ID, Transition{
		From:       StatusRunning,
		To:         StatusFailed,
		Phase:      PhaseCloning,
		FinishedAt: &finished,
		Error:      &OperationError{Code: "canceled_by_operator", Message: "Duplicate did not finish."},
	}); err != nil {
		t.Fatal(err)
	}
	second, same := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), second, same); err != nil {
		t.Fatalf("locks should be released after terminal: %v", err)
	}
}
