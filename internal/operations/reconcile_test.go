package operations

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProbe struct {
	outcome ProbeOutcome
	err     error
	calls   int
}

func (f *fakeProbe) DuplicateState(ctx context.Context, op Operation) (ProbeOutcome, error) {
	f.calls++
	if f.err != nil {
		return ProbeOutcome{}, f.err
	}
	return f.outcome, nil
}

func TestReconcileFlipsRunningToInterruptedThenProbes(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), op.ID, Transition{From: StatusQueued, To: StatusRunning, Phase: PhaseCloning}); err != nil {
		t.Fatal(err)
	}
	probe := &fakeProbe{outcome: ProbeOutcome{}}
	now := time.Date(2026, 8, 26, 6, 0, 0, 0, time.UTC)
	if err := store.Reconcile(context.Background(), probe, now); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed {
		t.Fatalf("nothing created should fail, status = %q", got.Status)
	}
	if probe.calls != 1 {
		t.Fatalf("probe calls = %d", probe.calls)
	}
}

func TestReconcileQueuedStaysQueued(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	if err := store.Reconcile(context.Background(), &fakeProbe{}, time.Date(2026, 8, 26, 6, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusQueued {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestReconcileNilProbeMarksIndeterminate(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), op.ID, Transition{From: StatusQueued, To: StatusRunning}); err != nil {
		t.Fatal(err)
	}
	if err := store.Reconcile(context.Background(), nil, time.Date(2026, 8, 26, 6, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusIndeterminate {
		t.Fatalf("nil probe status = %q", got.Status)
	}
}

func TestReconcileProbeOutcomes(t *testing.T) {
	now := time.Date(2026, 8, 26, 6, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		outcome ProbeOutcome
		err     error
		want    Status
	}{
		{name: "nothing_created", outcome: ProbeOutcome{}, want: StatusFailed},
		{name: "complete", outcome: ProbeOutcome{CloneExists: true, RoleExists: true, VaultRowExists: true}, want: StatusSucceeded},
		{name: "partial", outcome: ProbeOutcome{CloneExists: true}, want: StatusFailed},
		{name: "indeterminate_flag", outcome: ProbeOutcome{Indeterminate: true}, want: StatusIndeterminate},
		{name: "probe_error", err: errors.New("probe unavailable"), want: StatusIndeterminate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStore(t)
			op, locks := queuedDuplicate(t, "src_"+tc.name, "dst_"+tc.name, "role_"+tc.name)
			if err := store.InsertQueued(context.Background(), op, locks); err != nil {
				t.Fatal(err)
			}
			if err := store.Transition(context.Background(), op.ID, Transition{From: StatusQueued, To: StatusRunning, Phase: PhaseCloning}); err != nil {
				t.Fatal(err)
			}
			if err := store.Transition(context.Background(), op.ID, Transition{From: StatusRunning, To: StatusInterrupted}); err != nil {
				t.Fatal(err)
			}
			probe := &fakeProbe{outcome: tc.outcome, err: tc.err}
			if err := store.Reconcile(context.Background(), probe, now); err != nil {
				t.Fatal(err)
			}
			got, err := store.Get(context.Background(), op.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.want {
				t.Fatalf("status = %q, want %q", got.Status, tc.want)
			}
			if tc.want == StatusSucceeded {
				if got.Result == nil || got.Result.Database != op.Target || got.Result.Source != "src_"+tc.name || got.Result.Owner != "role_"+tc.name {
					t.Fatalf("result = %#v", got.Result)
				}
			}
		})
	}
}

func TestReconcileResumesCompensatingToFailed(t *testing.T) {
	store := newTestStore(t)
	op, locks := queuedDuplicate(t, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), op.ID, Transition{From: StatusQueued, To: StatusRunning}); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), op.ID, Transition{From: StatusRunning, To: StatusCompensating, Phase: PhaseCompensating}); err != nil {
		t.Fatal(err)
	}
	probe := &fakeProbe{outcome: ProbeOutcome{CloneExists: true, RoleExists: true, VaultRowExists: true}}
	if err := store.Reconcile(context.Background(), probe, time.Date(2026, 8, 26, 6, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed {
		t.Fatalf("compensating resume status = %q", got.Status)
	}
	if probe.calls != 0 {
		t.Fatalf("compensating must not probe CREATE DATABASE: calls=%d", probe.calls)
	}
}

func TestReconcilePrunesOldTerminalAndKeepsNonTerminal(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 8, 26, 6, 0, 0, 0, time.UTC)
	oldFinish := now.Add(-TerminalRetention - time.Hour)
	freshFinish := now.Add(-time.Hour)

	oldOp, oldLocks := queuedDuplicate(t, "old_src", "old_dst", "old_role")
	if err := store.InsertQueued(context.Background(), oldOp, oldLocks); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), oldOp.ID, Transition{From: StatusQueued, To: StatusCanceled, FinishedAt: &oldFinish}); err != nil {
		t.Fatal(err)
	}

	freshOp, freshLocks := queuedDuplicate(t, "fresh_src", "fresh_dst", "fresh_role")
	if err := store.InsertQueued(context.Background(), freshOp, freshLocks); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), freshOp.ID, Transition{From: StatusQueued, To: StatusCanceled, FinishedAt: &freshFinish}); err != nil {
		t.Fatal(err)
	}

	queuedOp, queuedLocks := queuedDuplicate(t, "queued_src", "queued_dst", "queued_role")
	queuedOp.CreatedAt = now.Add(-TerminalRetention - 2*time.Hour)
	queuedOp.UpdatedAt = queuedOp.CreatedAt
	if err := store.InsertQueued(context.Background(), queuedOp, queuedLocks); err != nil {
		t.Fatal(err)
	}

	if err := store.Reconcile(context.Background(), &fakeProbe{}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), oldOp.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old terminal should be pruned: %v", err)
	}
	if _, err := store.Get(context.Background(), freshOp.ID); err != nil {
		t.Fatalf("fresh terminal pruned: %v", err)
	}
	got, err := store.Get(context.Background(), queuedOp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusQueued {
		t.Fatalf("non-terminal pruned: %#v", got)
	}
}

func TestReconcileCapsTerminalRowsByOldestFinishedAt(t *testing.T) {
	db, store := newTestDB(t)
	now := time.Date(2026, 8, 26, 6, 0, 0, 0, time.UTC)
	const extra = 3
	ids := make([]string, MaxTerminalOperations+extra)
	for i := range ids {
		id := mustID(t)
		ids[i] = id
		finished := now.Add(-time.Duration(len(ids)-i) * time.Minute).Format(time.RFC3339Nano)
		created := now.Add(-time.Hour).Format(time.RFC3339Nano)
		_, err := db.Exec(`
INSERT INTO operations (id, action, status, actor, accepted_request_id, target, phase, created_at, updated_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, string(ActionDuplicate), string(StatusFailed), "admin", mustID(t), "db", string(PhaseCloning),
			created, created, finished,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Reconcile(context.Background(), &fakeProbe{}, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM operations WHERE status IN ('succeeded','failed','canceled','indeterminate')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != MaxTerminalOperations {
		t.Fatalf("terminal count = %d, want %d", count, MaxTerminalOperations)
	}
	if _, err := store.Get(context.Background(), ids[0]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("oldest terminal should be capped: %v", err)
	}
	if _, err := store.Get(context.Background(), ids[len(ids)-1]); err != nil {
		t.Fatalf("newest terminal missing: %v", err)
	}
}
