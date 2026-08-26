package postgresadmin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
)

func dropCatalog() *MemoryCatalog {
	return &MemoryCatalog{
		Rows: []CatalogRow{
			projectRow("project_a", "app_project_a"),
			projectRow("postgres", "postgres"),
			{Name: "closed_db", Owner: "app_project_a", AllowConn: false},
			{Name: "templ_db", Owner: "app_project_a", AllowConn: true, IsTemplate: true},
		},
		OwnedCount: 0,
	}
}

func TestFormatOperatorDropDatabaseQuotedNoIfExists(t *testing.T) {
	got, err := formatOperatorDropDatabase("project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got != `DROP DATABASE "project_a"` {
		t.Fatalf("got %s", got)
	}
	if strings.Contains(strings.ToUpper(got), "IF EXISTS") {
		t.Fatal("operator DROP DATABASE must not use IF EXISTS")
	}
	if strings.Contains(strings.ToUpper(got), "FORCE") || strings.Contains(strings.ToUpper(got), "CASCADE") {
		t.Fatalf("forbidden clause in %s", got)
	}
	comp, err := formatDropDatabase("project_a")
	if err != nil {
		t.Fatal(err)
	}
	if comp != `DROP DATABASE IF EXISTS "project_a"` {
		t.Fatalf("compensation sql = %s", comp)
	}
	if _, err := formatOperatorDropDatabase(""); err == nil {
		t.Fatal("empty name must fail closed")
	}
	if _, err := formatOperatorDropDatabase("a\x00b"); err == nil {
		t.Fatal("NUL name must fail closed")
	}
}

func TestFormatDropRoleKeepsIfExists(t *testing.T) {
	got, err := formatDropRole("app_project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got != `DROP ROLE IF EXISTS "app_project_a"` {
		t.Fatalf("got %s", got)
	}
}

func TestTerminateSQLExcludesCurrentBackendAndHasNoForce(t *testing.T) {
	if terminateDatabaseSQL != `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()` {
		t.Fatalf("sql = %s", terminateDatabaseSQL)
	}
	upper := strings.ToUpper(terminateDatabaseSQL)
	if strings.Contains(upper, "FORCE") {
		t.Fatal("terminate must not use FORCE")
	}
}

func TestServiceDropProtectedNoSQL(t *testing.T) {
	cat := dropCatalog()
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres"}))
	if _, err := svc.Drop(context.Background(), "postgres"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("protected: %v", err)
	}
	if cat.TerminateCalls != 0 || cat.DropCalls != 0 || cat.DropDatabaseCalls != 0 || cat.DropRoleCalls != 0 {
		t.Fatalf("SQL on protected: terminate=%d drop=%d compensate=%d role=%d", cat.TerminateCalls, cat.DropCalls, cat.DropDatabaseCalls, cat.DropRoleCalls)
	}
}

func TestServiceDropDisallowConnAndTemplateNoSQL(t *testing.T) {
	cat := dropCatalog()
	svc := NewService(cat, NewPolicy(config.Config{}))
	if _, err := svc.Drop(context.Background(), "closed_db"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("allowconn: %v", err)
	}
	if _, err := svc.Drop(context.Background(), "templ_db"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("template: %v", err)
	}
	if cat.TerminateCalls != 0 || cat.DropCalls != 0 {
		t.Fatalf("SQL on unmanageable: terminate=%d drop=%d", cat.TerminateCalls, cat.DropCalls)
	}
}

func TestServiceDropMissingNoSQL(t *testing.T) {
	cat := dropCatalog()
	svc := NewService(cat, NewPolicy(config.Config{}))
	if _, err := svc.Drop(context.Background(), "missing_db"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if cat.TerminateCalls != 0 || cat.DropCalls != 0 {
		t.Fatalf("SQL on missing: terminate=%d drop=%d", cat.TerminateCalls, cat.DropCalls)
	}
}

func TestServiceDropAfterValidationSkipsPreconditionForProtectedTarget(t *testing.T) {
	cat := dropCatalog()
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres"}))
	called := false
	_, err := svc.DropAfterValidation(context.Background(), "postgres", func(context.Context) error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("protected: %v", err)
	}
	if called {
		t.Fatal("precondition ran for protected target")
	}
	if cat.TerminateCalls != 0 || cat.DropCalls != 0 {
		t.Fatalf("SQL on protected target: terminate=%d drop=%d", cat.TerminateCalls, cat.DropCalls)
	}
}

func TestServiceDropAfterValidationRechecksTargetBeforeSQL(t *testing.T) {
	cat := dropCatalog()
	svc := NewService(cat, NewPolicy(config.Config{}))
	_, err := svc.DropAfterValidation(context.Background(), "project_a", func(context.Context) error {
		cat.Rows = nil
		return nil
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("final target re-read: %v", err)
	}
	if cat.TerminateCalls != 0 || cat.DropCalls != 0 {
		t.Fatalf("SQL after target changed: terminate=%d drop=%d", cat.TerminateCalls, cat.DropCalls)
	}
}

func TestServiceDropAfterValidationHoldsDropLock(t *testing.T) {
	cat := dropCatalog()
	svc := NewService(cat, NewPolicy(config.Config{}))
	started := make(chan struct{})
	release := make(chan struct{})
	callbackErr := errors.New("precondition stopped")
	done := make(chan error, 1)
	go func() {
		_, err := svc.DropAfterValidation(context.Background(), "project_a", func(context.Context) error {
			close(started)
			<-release
			return callbackErr
		})
		done <- err
	}()
	<-started
	var inProgress DropInProgress
	if _, err := svc.Drop(context.Background(), "project_a"); !errors.As(err, &inProgress) {
		close(release)
		t.Fatalf("concurrent drop: %v", err)
	}
	close(release)
	if err := <-done; !errors.Is(err, callbackErr) {
		t.Fatalf("callback error = %v", err)
	}
	if cat.TerminateCalls != 0 || cat.DropCalls != 0 {
		t.Fatalf("SQL after callback failure: terminate=%d drop=%d", cat.TerminateCalls, cat.DropCalls)
	}
}

func TestServiceDropOwnedCountNonZeroSkipsRoleAndVault(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 2
	svc := NewService(cat, NewPolicy(config.Config{}))
	got, err := svc.Drop(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Dropped != "project_a" || got.Owner != "app_project_a" || got.DroppedRole != "" {
		t.Fatalf("got = %#v", got)
	}
	if cat.TerminateCalls != 1 || cat.LastTerminateName != "project_a" {
		t.Fatalf("terminate = %d %q", cat.TerminateCalls, cat.LastTerminateName)
	}
	if cat.DropCalls != 1 || cat.DropDatabaseCalls != 0 {
		t.Fatalf("operator drop=%d compensate=%d", cat.DropCalls, cat.DropDatabaseCalls)
	}
	if cat.LastDropSQL != `DROP DATABASE "project_a"` {
		t.Fatalf("sql = %s", cat.LastDropSQL)
	}
	if cat.DropRoleCalls != 0 || cat.DeleteCredentialCalls != 0 {
		t.Fatalf("role=%d vault=%d", cat.DropRoleCalls, cat.DeleteCredentialCalls)
	}
}

func TestServiceDropOwnedCountErrorSkipsRoleAndVault(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 0
	cat.OwnedCountErr = errors.New("postgresql://canary:secret@127.0.0.1/db")
	svc := NewService(cat, NewPolicy(config.Config{}))
	got, err := svc.Drop(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.DroppedRole != "" {
		t.Fatalf("got = %#v", got)
	}
	if cat.DropCalls != 1 || cat.DropRoleCalls != 0 || cat.DeleteCredentialCalls != 0 {
		t.Fatalf("drop=%d role=%d vault=%d", cat.DropCalls, cat.DropRoleCalls, cat.DeleteCredentialCalls)
	}
}

func TestServiceDropProtectedOwnerNoSQL(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:       []CatalogRow{projectRow("project_a", "postgres")},
		OwnedCount: 0,
	}
	svc := NewService(cat, NewPolicy(config.Config{}))
	if _, err := svc.Drop(context.Background(), "project_a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("protected owner: %v", err)
	}
	if cat.TerminateCalls != 0 || cat.DropCalls != 0 || cat.DropRoleCalls != 0 {
		t.Fatalf("SQL on protected owner: terminate=%d drop=%d role=%d", cat.TerminateCalls, cat.DropCalls, cat.DropRoleCalls)
	}
}

func TestServiceDropOwnerDeniedSkipsRoleAndVault(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 0
	svc := NewService(cat, NewPolicy(config.Config{}))
	got, err := svc.dropOwnerIfUnowned(context.Background(), "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("dropped role = %q", got)
	}
	if cat.DropRoleCalls != 0 || cat.DeleteCredentialCalls != 0 {
		t.Fatalf("role=%d vault=%d", cat.DropRoleCalls, cat.DeleteCredentialCalls)
	}
}

func TestServiceDropCountZeroDropsRoleAndVault(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 0
	svc := NewService(cat, NewPolicy(config.Config{}))
	got, err := svc.Drop(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Dropped != "project_a" || got.DroppedRole != "app_project_a" || got.Owner != "app_project_a" {
		t.Fatalf("got = %#v", got)
	}
	if cat.DropCalls != 1 || cat.DropDatabaseCalls != 0 {
		t.Fatalf("operator drop=%d compensate=%d", cat.DropCalls, cat.DropDatabaseCalls)
	}
	if cat.DropRoleCalls != 1 || cat.DroppedRoles[0] != "app_project_a" {
		t.Fatalf("role = %#v calls=%d", cat.DroppedRoles, cat.DropRoleCalls)
	}
	if cat.DeleteCredentialCalls != 1 || cat.DeletedVault[0] != "app_project_a" {
		t.Fatalf("vault = %#v calls=%d", cat.DeletedVault, cat.DeleteCredentialCalls)
	}
}

func TestServiceDropRoleFailureAfterDatabase(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 0
	cat.DropRoleErr = errors.New("postgresql://canary:secret@127.0.0.1/db")
	svc := NewService(cat, NewPolicy(config.Config{}))
	got, err := svc.Drop(context.Background(), "project_a")
	var failed RoleDropFailed
	if !errors.As(err, &failed) || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if failed.Error() != roleDropFailedMessage {
		t.Fatalf("message = %q", failed.Error())
	}
	if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("leaked %v", err)
	}
	if got.Dropped != "project_a" {
		t.Fatalf("database must stay dropped: %#v", got)
	}
	if cat.DropCalls != 1 || cat.DropRoleCalls != 1 || cat.DeleteCredentialCalls != 0 {
		t.Fatalf("drop=%d role=%d vault=%d", cat.DropCalls, cat.DropRoleCalls, cat.DeleteCredentialCalls)
	}
}

func TestServiceDropVaultFailureAfterRole(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 0
	cat.DeleteCredentialErr = errors.New("postgresql://canary:secret@127.0.0.1/db")
	svc := NewService(cat, NewPolicy(config.Config{}))
	got, err := svc.Drop(context.Background(), "project_a")
	var failed VaultDeleteFailed
	if !errors.As(err, &failed) || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if failed.Error() != vaultDeleteFailedMessage {
		t.Fatalf("message = %q", failed.Error())
	}
	if strings.Contains(err.Error(), "canary") {
		t.Fatalf("leaked %v", err)
	}
	if got.Dropped != "project_a" {
		t.Fatalf("database must stay dropped: %#v", got)
	}
	if cat.DropCalls != 1 || cat.DropRoleCalls != 1 || cat.DeleteCredentialCalls != 1 {
		t.Fatalf("drop=%d role=%d vault=%d", cat.DropCalls, cat.DropRoleCalls, cat.DeleteCredentialCalls)
	}
}

func TestServiceDropTerminateFailureNoDrop(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 0
	cat.TerminateErr = errors.New("postgresql://canary:secret@127.0.0.1/db")
	svc := NewService(cat, NewPolicy(config.Config{}))
	_, err := svc.Drop(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary") {
		t.Fatalf("leaked %v", err)
	}
	if cat.TerminateCalls != 1 || cat.DropCalls != 0 || cat.DropRoleCalls != 0 {
		t.Fatalf("terminate=%d drop=%d role=%d", cat.TerminateCalls, cat.DropCalls, cat.DropRoleCalls)
	}
}

func TestServiceDropMapsCanaryErrors(t *testing.T) {
	cat := dropCatalog()
	cat.OwnedCount = 0
	cat.DropErr = errors.New("postgresql://canary:secret@127.0.0.1/db")
	svc := NewService(cat, NewPolicy(config.Config{}))
	_, err := svc.Drop(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("leaked %v", err)
	}
	if cat.DropCalls != 1 || cat.DropRoleCalls != 0 {
		t.Fatalf("drop=%d role=%d", cat.DropCalls, cat.DropRoleCalls)
	}
}

func TestServiceDropLockConflictNoSecondSQL(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:        []CatalogRow{projectRow("project_a", "app_project_a")},
		OwnedCount:  1,
		DropStarted: make(chan struct{}, 1),
		DropHold:    make(chan struct{}),
	}
	svc := NewService(cat, NewPolicy(config.Config{}))
	done := make(chan error, 1)
	go func() {
		_, err := svc.Drop(context.Background(), "project_a")
		done <- err
	}()
	select {
	case <-cat.DropStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first drop did not reach SQL")
	}
	_, err := svc.Drop(context.Background(), "project_a")
	var inProgress DropInProgress
	if !errors.As(err, &inProgress) || !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("second err = %v", err)
	}
	if inProgress.Error() != dropInProgressMessage {
		t.Fatalf("message = %q", inProgress.Error())
	}
	close(cat.DropHold)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("first drop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first drop blocked")
	}
	if cat.DropCalls != 1 {
		t.Fatalf("DropCalls = %d", cat.DropCalls)
	}
}

func TestServiceDropRefusesWhenTruncateHeld(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:            []CatalogRow{projectRow("project_a", "app_project_a")},
		Tables:          map[string][]TableItem{"project_a": {{Schema: "public", Name: "items"}}},
		OwnedCount:      1,
		TruncateStarted: make(chan struct{}, 1),
		TruncateHold:    make(chan struct{}),
	}
	svc := NewService(cat, NewPolicy(config.Config{}))
	done := make(chan error, 1)
	go func() {
		_, err := svc.Truncate(context.Background(), "project_a")
		done <- err
	}()
	select {
	case <-cat.TruncateStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("truncate did not reach SQL")
	}
	_, err := svc.Drop(context.Background(), "project_a")
	var inProgress TruncateInProgress
	if !errors.As(err, &inProgress) {
		t.Fatalf("drop err = %v", err)
	}
	if inProgress.Error() != truncateInProgressMessage {
		t.Fatalf("message = %q", inProgress.Error())
	}
	close(cat.TruncateHold)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("truncate: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("truncate blocked")
	}
	if cat.DropCalls != 0 || cat.TerminateCalls != 0 {
		t.Fatalf("drop SQL while truncate held: drop=%d terminate=%d", cat.DropCalls, cat.TerminateCalls)
	}
}

func TestServiceTruncateRefusesWhenDropHeld(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:        []CatalogRow{projectRow("project_a", "app_project_a")},
		Tables:      map[string][]TableItem{"project_a": {{Schema: "public", Name: "items"}}},
		OwnedCount:  1,
		DropStarted: make(chan struct{}, 1),
		DropHold:    make(chan struct{}),
	}
	svc := NewService(cat, NewPolicy(config.Config{}))
	done := make(chan error, 1)
	go func() {
		_, err := svc.Drop(context.Background(), "project_a")
		done <- err
	}()
	select {
	case <-cat.DropStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("drop did not reach SQL")
	}
	_, err := svc.Truncate(context.Background(), "project_a")
	var inProgress DropInProgress
	if !errors.As(err, &inProgress) {
		t.Fatalf("truncate err = %v", err)
	}
	if inProgress.Error() != dropInProgressMessage {
		t.Fatalf("message = %q", inProgress.Error())
	}
	close(cat.DropHold)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("drop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("drop blocked")
	}
	if cat.TruncateCalls != 0 {
		t.Fatalf("truncate SQL while drop held: %d", cat.TruncateCalls)
	}
}
