package postgresadmin

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/internal/operations"
	"github.com/SSujitX/redgres/migrations"
)

type fakeDuplicateAuditor struct {
	err      error
	calls    int
	metadata map[string]any
}

func (f *fakeDuplicateAuditor) Record(_, _, _, _, _, _ string, metadata map[string]any) error {
	f.calls++
	f.metadata = metadata
	return f.err
}

func TestRunQueuedDuplicatesSucceedsAndAuditsOperationID(t *testing.T) {
	store := newDuplicateStore(t)
	cat := &MemoryCatalog{Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")}}
	svc := duplicateService(t, cat, createVaultKey(t))
	op := insertQueuedDuplicateOp(t, store, "project_a", "project_a_copy", "app_project_a_copy")
	auditor := &fakeDuplicateAuditor{}
	if err := RunQueuedDuplicates(context.Background(), store, svc, auditor); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != operations.StatusSucceeded {
		t.Fatalf("status = %q", got.Status)
	}
	if got.Result == nil || got.Result.Database != "project_a_copy" || got.Result.Owner != "app_project_a_copy" || got.Result.Source != "project_a" {
		t.Fatalf("result = %#v", got.Result)
	}
	if cat.CreateDatabaseTemplateCalls != 1 || cat.InsertCalls != 1 {
		t.Fatalf("template/insert = %d/%d", cat.CreateDatabaseTemplateCalls, cat.InsertCalls)
	}
	if auditor.calls != 1 {
		t.Fatalf("audit calls = %d", auditor.calls)
	}
	if auditor.metadata["operation_id"] != op.ID || auditor.metadata["database"] != "project_a_copy" || auditor.metadata["owner"] != "app_project_a_copy" || auditor.metadata["source"] != "project_a" {
		t.Fatalf("metadata = %#v", auditor.metadata)
	}
}

func TestRunQueuedDuplicatesVaultMissingFailsBeforeDDL(t *testing.T) {
	store := newDuplicateStore(t)
	cat := &MemoryCatalog{Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")}}
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres", PostgresUser: "redgres_console"}))
	op := insertQueuedDuplicateOp(t, store, "project_a", "project_a_copy", "app_project_a_copy")
	if err := RunQueuedDuplicates(context.Background(), store, svc, &fakeDuplicateAuditor{}); err != nil {
		t.Fatal(err)
	}
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 {
		t.Fatal("missing vault key must not DDL")
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != operations.StatusFailed {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestRunQueuedDuplicatesIsolationMismatchCompensates(t *testing.T) {
	store := newDuplicateStore(t)
	cat := &MemoryCatalog{
		Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")},
		SnapshotSeq: []OwnershipSnapshot{
			{Owner: "app_project_a", Datacl: ""},
			{Owner: "other_role", Datacl: "{changed}"},
		},
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	op := insertQueuedDuplicateOp(t, store, "project_a", "project_a_copy", "app_project_a_copy")
	if err := RunQueuedDuplicates(context.Background(), store, svc, &fakeDuplicateAuditor{}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != operations.StatusFailed {
		t.Fatalf("status = %q", got.Status)
	}
	if got.Error == nil || got.Error.Message != isolationChangedMessage {
		t.Fatalf("error = %#v", got.Error)
	}
	if len(cat.DroppedDatabases) == 0 || cat.DroppedDatabases[0] != "project_a_copy" {
		t.Fatalf("dropped = %#v", cat.DroppedDatabases)
	}
	if containsString(cat.DroppedDatabases, "project_a") {
		t.Fatal("must not drop source")
	}
}

func TestRunQueuedDuplicatesAuditFailureKeepsSucceeded(t *testing.T) {
	store := newDuplicateStore(t)
	cat := &MemoryCatalog{Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")}}
	svc := duplicateService(t, cat, createVaultKey(t))
	op := insertQueuedDuplicateOp(t, store, "project_a", "project_a_copy", "app_project_a_copy")
	auditor := &fakeDuplicateAuditor{err: errors.New("audit down")}
	if err := RunQueuedDuplicates(context.Background(), store, svc, auditor); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != operations.StatusSucceeded {
		t.Fatalf("status = %q", got.Status)
	}
	if cat.DropDatabaseCalls != 0 || cat.DropRoleCalls != 0 {
		t.Fatal("audit-fail must not compensate a complete clone")
	}
	if cat.InsertCalls != 1 {
		t.Fatalf("inserts = %d", cat.InsertCalls)
	}
}

func TestRunQueuedDuplicatesDoesNotRetryInterrupted(t *testing.T) {
	store := newDuplicateStore(t)
	cat := &MemoryCatalog{Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")}}
	svc := duplicateService(t, cat, createVaultKey(t))
	op := insertQueuedDuplicateOp(t, store, "project_a", "project_a_copy", "app_project_a_copy")
	if err := store.Transition(context.Background(), op.ID, operations.Transition{From: operations.StatusQueued, To: operations.StatusRunning}); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), op.ID, operations.Transition{From: operations.StatusRunning, To: operations.StatusInterrupted}); err != nil {
		t.Fatal(err)
	}
	if err := RunQueuedDuplicates(context.Background(), store, svc, &fakeDuplicateAuditor{}); err != nil {
		t.Fatal(err)
	}
	if cat.CreateDatabaseTemplateCalls != 0 {
		t.Fatal("must not retry CREATE DATABASE from interrupted")
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != operations.StatusInterrupted {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestPrepareDuplicateConflictsExistingVaultRole(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:       []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")},
		SavedRoles: []string{"app_project_a_copy"},
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	err := svc.PrepareDuplicate(context.Background(), "project_a", "project_a_copy", "app_project_a_copy")
	var conflict Conflict
	if !errors.As(err, &conflict) || conflict.Field != conflictFieldOwner {
		t.Fatalf("err = %v", err)
	}
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 || cat.DeleteCredentialCalls != 0 {
		t.Fatal("existing vault name must not DDL or delete")
	}
}

func TestDuplicateCompensatorKeepsOtherProjectVault(t *testing.T) {
	cat := &MemoryCatalog{
		Rows: []CatalogRow{
			eligibleDuplicateRow("project_a", "app_project_a"),
			eligibleDuplicateRow("other_db", "app_project_a_copy"),
		},
		CreatedRoles: []string{"app_project_a_copy"},
		SavedRoles:   []string{"app_project_a_copy"},
		OwnedCount:   1,
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	err := NewDuplicateCompensator(svc).CompensateDuplicate(context.Background(), operations.Operation{
		Result: &operations.DuplicateResult{Database: "project_a_copy", Owner: "app_project_a_copy", Source: "project_a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cat.DeleteCredentialCalls != 0 {
		t.Fatalf("must not delete other project vault: %d", cat.DeleteCredentialCalls)
	}
	if cat.DropRoleCalls != 0 {
		t.Fatalf("must not drop other project role: %d", cat.DropRoleCalls)
	}
	if containsString(cat.DroppedDatabases, "other_db") || containsString(cat.DroppedDatabases, "project_a") {
		t.Fatalf("dropped = %#v", cat.DroppedDatabases)
	}
}

func TestDuplicateCompensatorLeavesVaultOnlyRow(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:       []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")},
		SavedRoles: []string{"app_project_a_copy"},
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	err := NewDuplicateCompensator(svc).CompensateDuplicate(context.Background(), operations.Operation{
		Result: &operations.DuplicateResult{Database: "project_a_copy", Owner: "app_project_a_copy", Source: "project_a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cat.DeleteCredentialCalls != 0 {
		t.Fatalf("vault-only must not delete: %d", cat.DeleteCredentialCalls)
	}
}

func TestRunQueuedDuplicatesConflictDoesNotDeleteOtherVault(t *testing.T) {
	store := newDuplicateStore(t)
	cat := &MemoryCatalog{
		Rows: []CatalogRow{
			eligibleDuplicateRow("project_a", "app_project_a"),
			eligibleDuplicateRow("other_db", "app_project_a_copy"),
		},
		ExistingRoles: []string{"app_project_a_copy"},
		SavedRoles:    []string{"app_project_a_copy"},
		OwnedCount:    1,
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	op := insertQueuedDuplicateOp(t, store, "project_a", "project_a_copy", "app_project_a_copy")
	if err := RunQueuedDuplicates(context.Background(), store, svc, &fakeDuplicateAuditor{}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != operations.StatusFailed {
		t.Fatalf("status = %q", got.Status)
	}
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 {
		t.Fatal("conflict must not DDL")
	}
	if cat.DeleteCredentialCalls != 0 {
		t.Fatalf("must not delete other vault: %d", cat.DeleteCredentialCalls)
	}
	if cat.DropRoleCalls != 0 {
		t.Fatalf("must not drop other role: %d", cat.DropRoleCalls)
	}
}

func TestDuplicateCompensatorDropsLeftoversNotSource(t *testing.T) {
	cat := &MemoryCatalog{
		Rows: []CatalogRow{
			eligibleDuplicateRow("project_a", "app_project_a"),
			eligibleDuplicateRow("project_a_copy", "app_project_a_copy"),
		},
		CreatedRoles: []string{"app_project_a_copy"},
		SavedRoles:   []string{"app_project_a_copy"},
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	err := NewDuplicateCompensator(svc).CompensateDuplicate(context.Background(), operations.Operation{
		Result: &operations.DuplicateResult{Database: "project_a_copy", Owner: "app_project_a_copy", Source: "project_a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cat.DeleteCredentialCalls != 1 {
		t.Fatalf("vault deletes = %d", cat.DeleteCredentialCalls)
	}
	if len(cat.DroppedDatabases) != 1 || cat.DroppedDatabases[0] != "project_a_copy" {
		t.Fatalf("dropped dbs = %#v", cat.DroppedDatabases)
	}
	if containsString(cat.DroppedDatabases, "project_a") {
		t.Fatal("must not drop source")
	}
	if cat.DropRoleCalls != 1 || cat.DroppedRoles[0] != "app_project_a_copy" {
		t.Fatalf("dropped roles = %#v", cat.DroppedRoles)
	}
	if len(cat.EncryptedPasswordCalls) != 0 {
		t.Fatal("compensator must not decrypt")
	}
}

func TestDuplicateCompensatorErrorDoesNotLeak(t *testing.T) {
	canary := "postgresql://canary-token:secret@10.0.0.1/db"
	cat := &MemoryCatalog{
		Rows:                []CatalogRow{eligibleDuplicateRow("project_a_copy", "app_project_a_copy")},
		CreatedRoles:        []string{"app_project_a_copy"},
		SavedRoles:          []string{"app_project_a_copy"},
		DeleteCredentialErr: errors.New(canary),
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	err := NewDuplicateCompensator(svc).CompensateDuplicate(context.Background(), operations.Operation{
		Result: &operations.DuplicateResult{Database: "project_a_copy", Owner: "app_project_a_copy", Source: "project_a"},
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "canary-token") {
		t.Fatalf("leaked canary: %v", err)
	}
}

func TestRunQueuedDuplicatesCompensatorErrorKeepsLocks(t *testing.T) {
	store := newDuplicateStore(t)
	canary := "postgresql://canary-token:secret@10.0.0.1/db"
	cat := &MemoryCatalog{
		Rows:                []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")},
		TransferErr:         ErrUnavailable,
		DeleteCredentialErr: errors.New(canary),
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	op := insertQueuedDuplicateOp(t, store, "project_a", "project_a_copy", "app_project_a_copy")
	if err := RunQueuedDuplicates(context.Background(), store, svc, &fakeDuplicateAuditor{}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != operations.StatusIndeterminate {
		t.Fatalf("status = %q", got.Status)
	}
	second, locks := queuedDuplicateOp(t, "project_a", "other_copy", "app_other_copy")
	if err := store.InsertQueued(context.Background(), second, locks); !errors.Is(err, operations.ErrLockHeld) {
		t.Fatalf("indeterminate must keep locks: %v", err)
	}
}

func newDuplicateStore(t *testing.T) operations.Store {
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
	return operations.NewStore(db)
}

func insertQueuedDuplicateOp(t *testing.T, store operations.Store, source, database, owner string) operations.Operation {
	t.Helper()
	op, locks := queuedDuplicateOp(t, source, database, owner)
	if err := store.InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	return op
}

func queuedDuplicateOp(t *testing.T, source, database, owner string) (operations.Operation, []operations.ResourceLock) {
	t.Helper()
	id, err := operations.NewID()
	if err != nil {
		t.Fatal(err)
	}
	req, err := operations.NewID()
	if err != nil {
		t.Fatal(err)
	}
	op := operations.Operation{
		ID:                id,
		Action:            operations.ActionDuplicate,
		Actor:             "admin",
		Target:            database,
		AcceptedRequestID: req,
		Result: &operations.DuplicateResult{
			Database: database,
			Owner:    owner,
			Source:   source,
		},
	}
	locks := []operations.ResourceLock{
		{Kind: operations.ResourceDatabase, Name: source},
		{Kind: operations.ResourceDatabase, Name: database},
		{Kind: operations.ResourceRole, Name: owner},
	}
	return op, locks
}
