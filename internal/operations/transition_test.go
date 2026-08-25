package operations

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTransitionLegalEdges(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 8, 26, 5, 1, 0, 0, time.UTC)
	if err := store.Transition(context.Background(), op.ID, Transition{
		From:      StatusQueued,
		To:        StatusRunning,
		Phase:     PhaseCloning,
		StartedAt: &started,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusRunning || got.Phase != PhaseCloning {
		t.Fatalf("after claim: %#v", got)
	}
	if got.StartedAt == nil || !got.StartedAt.Equal(started) {
		t.Fatalf("started_at = %v", got.StartedAt)
	}

	finished := started.Add(2 * time.Minute)
	result := &DuplicateResult{Database: "project_a_copy", Owner: "app_project_a_copy", Source: "project_a"}
	if err := store.Transition(context.Background(), op.ID, Transition{
		From:       StatusRunning,
		To:         StatusSucceeded,
		Phase:      PhaseVaulting,
		Result:     result,
		FinishedAt: &finished,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusSucceeded || got.Result == nil || *got.Result != *result {
		t.Fatalf("after success: %#v", got)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finished) {
		t.Fatalf("finished_at = %v", got.FinishedAt)
	}
}

func TestTransitionIdempotentSameEdge(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	change := Transition{From: StatusQueued, To: StatusRunning, Phase: PhaseCloning}
	if err := store.Transition(context.Background(), op.ID, change); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), op.ID, change); err != nil {
		t.Fatalf("idempotent claim: %v", err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusRunning {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestTransitionIllegalEdgesLeaveRowUnchanged(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 8, 26, 5, 1, 0, 0, time.UTC)
	if err := store.Transition(context.Background(), op.ID, Transition{
		From:      StatusQueued,
		To:        StatusRunning,
		Phase:     PhaseCloning,
		StartedAt: &started,
	}); err != nil {
		t.Fatal(err)
	}
	before, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}

	illegal := []Transition{
		{From: StatusInterrupted, To: StatusRunning, Phase: PhaseCloning},
		{From: StatusQueued, To: StatusSucceeded},
		{From: StatusRunning, To: StatusQueued},
		{From: StatusRunning, To: StatusCanceled},
		{From: StatusSucceeded, To: StatusFailed},
	}
	for _, change := range illegal {
		err := store.Transition(context.Background(), op.ID, change)
		if !errors.Is(err, ErrIllegalEdge) {
			t.Fatalf("%s -> %s: got %v, want ErrIllegalEdge", change.From, change.To, err)
		}
		got, err := store.Get(context.Background(), op.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != before.Status || got.Phase != before.Phase {
			t.Fatalf("row changed on illegal %s -> %s: %#v", change.From, change.To, got)
		}
	}
}

func TestTransitionQueuedToCanceledOnly(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	finished := time.Date(2026, 8, 26, 5, 2, 0, 0, time.UTC)
	if err := store.Transition(context.Background(), op.ID, Transition{
		From:       StatusQueued,
		To:         StatusCanceled,
		FinishedAt: &finished,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusCanceled {
		t.Fatalf("status = %q", got.Status)
	}
	second, same := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), second, same); err != nil {
		t.Fatalf("canceled should release locks: %v", err)
	}
}

func TestTransitionInvalidID(t *testing.T) {
	store := newTestStore(t)
	err := store.Transition(context.Background(), "nope", Transition{From: StatusQueued, To: StatusRunning})
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("got %v, want ErrInvalidID", err)
	}
}

func TestTransitionMissingID(t *testing.T) {
	store := newTestStore(t)
	err := store.Transition(context.Background(), mustID(t), Transition{From: StatusQueued, To: StatusRunning})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestTransitionInterruptedNeverRetriesRunning(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), op.ID, Transition{From: StatusQueued, To: StatusRunning, Phase: PhaseCloning}); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), op.ID, Transition{From: StatusRunning, To: StatusInterrupted}); err != nil {
		t.Fatal(err)
	}
	err := store.Transition(context.Background(), op.ID, Transition{From: StatusInterrupted, To: StatusRunning})
	if !errors.Is(err, ErrIllegalEdge) {
		t.Fatalf("got %v, want ErrIllegalEdge", err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusInterrupted {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestTransitionCompensatingAndInterruptedTargets(t *testing.T) {
	store := newTestStore(t)
	cases := []struct {
		name string
		from Status
		to   Status
	}{
		{name: "compensating_failed", from: StatusCompensating, to: StatusFailed},
		{name: "compensating_indeterminate", from: StatusCompensating, to: StatusIndeterminate},
		{name: "interrupted_succeeded", from: StatusInterrupted, to: StatusSucceeded},
		{name: "interrupted_failed", from: StatusInterrupted, to: StatusFailed},
		{name: "interrupted_compensating", from: StatusInterrupted, to: StatusCompensating},
		{name: "interrupted_indeterminate", from: StatusInterrupted, to: StatusIndeterminate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op, locks := queuedDuplicate(t, "src_"+tc.name, "dst_"+tc.name, "role_"+tc.name)
			if err := store.InsertQueued(context.Background(), op, locks); err != nil {
				t.Fatal(err)
			}
			if err := store.Transition(context.Background(), op.ID, Transition{From: StatusQueued, To: StatusRunning, Phase: PhaseCloning}); err != nil {
				t.Fatal(err)
			}
			via := StatusInterrupted
			if tc.from == StatusCompensating {
				via = StatusCompensating
			}
			if err := store.Transition(context.Background(), op.ID, Transition{From: StatusRunning, To: via}); err != nil {
				t.Fatal(err)
			}
			change := Transition{From: tc.from, To: tc.to, Phase: PhaseCompensating}
			if tc.to == StatusSucceeded {
				change.Result = &DuplicateResult{Database: op.Target, Owner: "role_" + tc.name, Source: "src_" + tc.name}
			}
			if tc.to == StatusFailed || tc.to == StatusIndeterminate {
				change.Error = &OperationError{Code: "probe_incomplete", Message: "Duplicate did not finish."}
			}
			if err := store.Transition(context.Background(), op.ID, change); err != nil {
				t.Fatal(err)
			}
			got, err := store.Get(context.Background(), op.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.to {
				t.Fatalf("status = %q", got.Status)
			}
		})
	}
}
