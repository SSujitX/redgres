package postgresadmin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/secrets"
)

func eligibleDuplicateRow(name, owner string) CatalogRow {
	return CatalogRow{Name: name, Owner: owner, AllowConn: true, OwnerCanLogin: true}
}

func duplicateService(t *testing.T, cat *MemoryCatalog, vaultKey string) *Service {
	t.Helper()
	return createService(t, cat, config.Config{}, vaultKey)
}

func TestCreateDatabaseTemplateSQLShape(t *testing.T) {
	got, err := formatCreateDatabaseTemplate("project_a_copy", "project_a", "app_project_a_copy")
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE DATABASE "project_a_copy" TEMPLATE "project_a" OWNER "app_project_a_copy"`
	if got != want {
		t.Fatalf("sql = %s", got)
	}
	if strings.Contains(strings.ToLower(got), "reassign") {
		t.Fatal("TEMPLATE SQL must not REASSIGN")
	}
}

func TestTerminateSessionsSQLIsParameterizedOnDatname(t *testing.T) {
	if terminateDatabaseSQL != `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()` {
		t.Fatalf("sql = %s", terminateDatabaseSQL)
	}
	if strings.Contains(terminateDatabaseSQL, "datname = '") {
		t.Fatal("datname must be parameterized")
	}
}

func TestDuplicatePackageNeverUsesReassignOwnedOrSetRole(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(".", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		src := string(body)
		upper := strings.ToUpper(src)
		if strings.Contains(upper, "REASSIGN OWNED") {
			t.Fatalf("%s contains REASSIGN OWNED", entry.Name())
		}
		if strings.Contains(src, "SET ROLE") {
			t.Fatalf("%s contains SET ROLE", entry.Name())
		}
		if strings.Contains(src, "RESET ROLE") {
			t.Fatalf("%s contains RESET ROLE", entry.Name())
		}
	}
	blob := cloneNamespaceSQL + cloneRelationSQL
	if strings.Contains(blob, "LIKE 'pg") {
		t.Fatal("transfer SQL must not use LIKE 'pg")
	}
	if !strings.Contains(cloneNamespaceSQL, "nspname !~") || !strings.Contains(cloneRelationSQL, "nspname !~") {
		t.Fatal("transfer SQL must use nspname !~")
	}
}

func TestServiceDuplicateUniqueOwnerBeforeDDL(t *testing.T) {
	cat := &MemoryCatalog{Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")}}
	svc := duplicateService(t, cat, createVaultKey(t))
	_, err := svc.Duplicate(context.Background(), "project_a", "project_a_copy", "app_project_a")
	var field FieldError
	if !errors.As(err, &field) || field.Field != conflictFieldOwner || field.Message != duplicateSameOwnerMessage {
		t.Fatalf("err = %v", err)
	}
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 || cat.TerminateCalls != 0 {
		t.Fatal("unique owner must reject before DDL")
	}
}

func TestServiceDuplicateConflictDatabaseAndRole(t *testing.T) {
	key := createVaultKey(t)
	t.Run("database", func(t *testing.T) {
		cat := &MemoryCatalog{Rows: []CatalogRow{
			eligibleDuplicateRow("project_a", "app_project_a"),
			eligibleDuplicateRow("project_a_copy", "app_other"),
		}}
		svc := duplicateService(t, cat, key)
		_, err := svc.Duplicate(context.Background(), "project_a", "project_a_copy", "app_project_a_copy")
		var conflict Conflict
		if !errors.As(err, &conflict) || conflict.Field != conflictFieldDatabase {
			t.Fatalf("err = %v", err)
		}
		if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 {
			t.Fatal("conflict must not DDL")
		}
	})
	t.Run("role", func(t *testing.T) {
		cat := &MemoryCatalog{
			Rows:          []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")},
			ExistingRoles: []string{"app_project_a_copy"},
		}
		svc := duplicateService(t, cat, key)
		_, err := svc.Duplicate(context.Background(), "project_a", "project_a_copy", "app_project_a_copy")
		var conflict Conflict
		if !errors.As(err, &conflict) || conflict.Field != conflictFieldOwner {
			t.Fatalf("err = %v", err)
		}
		if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 {
			t.Fatal("conflict must not DDL")
		}
	})
}

func TestServiceDuplicateProtectedSourceAndNewName(t *testing.T) {
	key := createVaultKey(t)
	t.Run("protected-source", func(t *testing.T) {
		cat := &MemoryCatalog{Rows: []CatalogRow{eligibleDuplicateRow("postgres", "app_project_a")}}
		svc := duplicateService(t, cat, key)
		_, err := svc.Duplicate(context.Background(), "postgres", "project_a_copy", "app_project_a_copy")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
		if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 {
			t.Fatal("protected source must not DDL")
		}
	})
	t.Run("template-source", func(t *testing.T) {
		cat := &MemoryCatalog{Rows: []CatalogRow{{Name: "project_a", Owner: "app_project_a", AllowConn: true, IsTemplate: true, OwnerCanLogin: true}}}
		svc := duplicateService(t, cat, key)
		_, err := svc.Duplicate(context.Background(), "project_a", "project_a_copy", "app_project_a_copy")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
		if cat.CreateDatabaseTemplateCalls != 0 {
			t.Fatal("template source must not DDL")
		}
	})
	t.Run("missing-source", func(t *testing.T) {
		cat := &MemoryCatalog{}
		svc := duplicateService(t, cat, key)
		_, err := svc.Duplicate(context.Background(), "missing_db", "project_a_copy", "app_project_a_copy")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
		if cat.CreateDatabaseTemplateCalls != 0 {
			t.Fatal("missing source must not DDL")
		}
	})
	t.Run("superuser-source", func(t *testing.T) {
		cat := &MemoryCatalog{Rows: []CatalogRow{{Name: "project_a", Owner: "app_project_a", AllowConn: true, OwnerCanLogin: true, OwnerIsSuperuser: true}}}
		svc := duplicateService(t, cat, key)
		_, err := svc.Duplicate(context.Background(), "project_a", "project_a_copy", "app_project_a_copy")
		if !errors.Is(err, ErrProtected) {
			t.Fatalf("err = %v", err)
		}
		if cat.CreateDatabaseTemplateCalls != 0 {
			t.Fatal("superuser source must not DDL")
		}
	})
	t.Run("protected-new-database", func(t *testing.T) {
		cat := &MemoryCatalog{Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")}}
		svc := duplicateService(t, cat, key)
		_, err := svc.Duplicate(context.Background(), "project_a", "postgres", "app_project_a_copy")
		if !errors.Is(err, ErrProtected) {
			t.Fatalf("err = %v", err)
		}
		if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 {
			t.Fatal("protected new name must not DDL")
		}
	})
	t.Run("protected-new-owner", func(t *testing.T) {
		cat := &MemoryCatalog{Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")}}
		svc := duplicateService(t, cat, key)
		_, err := svc.Duplicate(context.Background(), "project_a", "project_a_copy", "postgres")
		if !errors.Is(err, ErrProtected) {
			t.Fatalf("err = %v", err)
		}
		if cat.CreateDatabaseTemplateCalls != 0 {
			t.Fatal("protected new owner must not DDL")
		}
	})
}

func TestServiceDuplicateMissingVaultKeyNoDDL(t *testing.T) {
	cat := &MemoryCatalog{Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")}}
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres", PostgresUser: "redgres_console"}))
	_, err := svc.Duplicate(context.Background(), "project_a", "project_a_copy", "app_project_a_copy")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseTemplateCalls != 0 || cat.TerminateCalls != 0 {
		t.Fatal("missing vault key must not DDL")
	}
}

func TestServiceDuplicateFingerprintMismatchCompensatesCloneOnly(t *testing.T) {
	cat := &MemoryCatalog{
		Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")},
		SnapshotSeq: []OwnershipSnapshot{
			{Owner: "app_project_a", Datacl: ""},
			{Owner: "other_role", Datacl: "{changed}"},
		},
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	_, err := svc.Duplicate(context.Background(), "project_a", "project_a_copy", "app_project_a_copy")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	var isolation IsolationChanged
	if !errors.As(err, &isolation) {
		t.Fatalf("want IsolationChanged, got %v", err)
	}
	if isolation.Error() != isolationChangedMessage {
		t.Fatalf("message = %s", isolation.Error())
	}
	if cat.CreateDatabaseTemplateCalls != 1 {
		t.Fatalf("template calls = %d", cat.CreateDatabaseTemplateCalls)
	}
	if cat.InsertCalls != 0 {
		t.Fatal("mismatch before vault must not insert")
	}
	if len(cat.DroppedDatabases) != 1 || cat.DroppedDatabases[0] != "project_a_copy" {
		t.Fatalf("dropped dbs = %#v", cat.DroppedDatabases)
	}
	for _, name := range cat.DroppedDatabases {
		if name == "project_a" {
			t.Fatal("compensation must not drop source")
		}
	}
	if cat.LastTerminateName == "project_a" && containsString(cat.DroppedDatabases, "project_a") {
		t.Fatal("must not drop source")
	}
	still, err := cat.Lookup(context.Background(), "project_a")
	if err != nil || still.Name != "project_a" {
		t.Fatalf("source missing after compensate: %v %#v", err, still)
	}
	if cat.DropRoleCalls != 1 || cat.DroppedRoles[0] != "app_project_a_copy" {
		t.Fatalf("dropped roles = %#v", cat.DroppedRoles)
	}
}

func TestServiceDuplicateVaultInsertFailureDropsCloneAndRoleOnly(t *testing.T) {
	canary := "postgresql://canary-token:secret@10.0.0.1/db"
	cat := &MemoryCatalog{
		Rows:                []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")},
		InsertCredentialErr: errors.New(canary),
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	_, err := svc.Duplicate(context.Background(), "project_a", "project_a_copy", "app_project_a_copy")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if err != nil && (strings.Contains(err.Error(), "canary-token") || strings.Contains(err.Error(), canary)) {
		t.Fatalf("leaked canary: %v", err)
	}
	if cat.CreateDatabaseCalls != 0 {
		t.Fatal("duplicate must not use create-without-template")
	}
	if cat.CreateDatabaseTemplateCalls != 1 || cat.LockConnectCalls != 1 {
		t.Fatalf("template/lock = %d/%d", cat.CreateDatabaseTemplateCalls, cat.LockConnectCalls)
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
	if containsString(cat.DroppedRoles, "app_project_a") {
		t.Fatal("must not drop source owner")
	}
}

func TestServiceDuplicateConcurrentLock(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:            []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")},
		TemplateStarted: make(chan struct{}, 1),
		TemplateHold:    make(chan struct{}),
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	done := make(chan error, 1)
	go func() {
		_, err := svc.Duplicate(context.Background(), "project_a", "project_a_copy", "app_project_a_copy")
		done <- err
	}()
	select {
	case <-cat.TemplateStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first duplicate did not reach TEMPLATE")
	}
	got, err := svc.Duplicate(context.Background(), "project_a", "project_b_copy", "app_project_b_copy")
	if !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("second err = %v", err)
	}
	var inProgress DuplicateInProgress
	if !errors.As(err, &inProgress) {
		t.Fatalf("want DuplicateInProgress, got %v", err)
	}
	if inProgress.Error() != duplicateInProgressMessage {
		t.Fatalf("message = %s", inProgress.Error())
	}
	if got.Password != "" {
		t.Fatal("409 must not return password")
	}
	close(cat.TemplateHold)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("first duplicate: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first duplicate blocked")
	}
	if cat.CreateDatabaseTemplateCalls != 1 {
		t.Fatalf("CreateDatabaseTemplateCalls = %d", cat.CreateDatabaseTemplateCalls)
	}
}

func TestServiceDuplicateSuccessUsesTemplateAndVaultInsert(t *testing.T) {
	cat := &MemoryCatalog{Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")}}
	key := createVaultKey(t)
	svc := duplicateService(t, cat, key)
	got, err := svc.Duplicate(context.Background(), "project_a", "project_a_copy", "app_project_a_copy")
	if err != nil {
		t.Fatal(err)
	}
	if got.Database != "project_a_copy" || got.Owner != "app_project_a_copy" || len(got.Password) != 32 {
		t.Fatalf("got = %#v", got)
	}
	if cat.CreateDatabaseCalls != 0 {
		t.Fatal("must not CREATE DATABASE without TEMPLATE")
	}
	if cat.CreateDatabaseTemplateCalls != 1 {
		t.Fatalf("template calls = %d", cat.CreateDatabaseTemplateCalls)
	}
	if cat.LastCreateDatabaseTemplateSQL != `CREATE DATABASE "project_a_copy" TEMPLATE "project_a" OWNER "app_project_a_copy"` {
		t.Fatalf("sql = %s", cat.LastCreateDatabaseTemplateSQL)
	}
	if cat.LastTerminateName != "project_a" {
		t.Fatalf("terminate = %s", cat.LastTerminateName)
	}
	if cat.InsertCalls != 1 || cat.UpsertCalls != 0 {
		t.Fatalf("insert=%d upsert=%d", cat.InsertCalls, cat.UpsertCalls)
	}
	if cat.TransferCalls != 1 || cat.LastTransferDB != "project_a_copy" {
		t.Fatalf("transfer = %d %s", cat.TransferCalls, cat.LastTransferDB)
	}
	plain, err := secrets.Decrypt(key, cat.Ciphertexts["app_project_a_copy"])
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != got.Password {
		t.Fatal("vault token must decrypt to generated password")
	}
}

func TestServiceDuplicateSkipProtectedOwners(t *testing.T) {
	if !skipCloneObjectOwner(NewPolicy(config.Config{PostgresUser: "redgres_console"}), "postgres") {
		t.Fatal("postgres owner must skip")
	}
	if !skipCloneObjectOwner(NewPolicy(config.Config{}), "pg_signal_backend") {
		t.Fatal("pg_* owner must skip")
	}
	if skipCloneObjectOwner(NewPolicy(config.Config{}), "app_project_a") {
		t.Fatal("project owner must not skip")
	}
}

func TestSkipCloneTransferOwnerSuperuser(t *testing.T) {
	skip := func(owner string) bool {
		return skipCloneObjectOwner(NewPolicy(config.Config{PostgresUser: "redgres_console"}), owner)
	}
	if !skipCloneTransferOwner(skip, "app_project_a", true) {
		t.Fatal("superuser object owner must skip")
	}
	if skipCloneTransferOwner(skip, "app_project_a", false) {
		t.Fatal("project owner must not skip")
	}
	if !skipCloneTransferOwner(skip, "postgres", false) {
		t.Fatal("denied owner must skip")
	}
}

func TestFormatAlterRelationOwnerQuotesCatalogNames(t *testing.T) {
	got, err := formatAlterRelationOwner("r", "public", "Order", "app_project_a_copy")
	if err != nil {
		t.Fatal(err)
	}
	if got != `ALTER TABLE "public"."Order" OWNER TO "app_project_a_copy"` {
		t.Fatalf("sql = %s", got)
	}
	hyphen, err := formatAlterRelationOwner("r", "public", "user-data", "app_project_a_copy")
	if err != nil {
		t.Fatal(err)
	}
	if hyphen != `ALTER TABLE "public"."user-data" OWNER TO "app_project_a_copy"` {
		t.Fatalf("sql = %s", hyphen)
	}
	if _, err := formatAlterRelationOwner("r", "public", "", "app_project_a_copy"); err == nil {
		t.Fatal("empty catalog name must fail closed")
	}
	if _, err := formatAlterRelationOwner("r", "public", "a\x00b", "app_project_a_copy"); err == nil {
		t.Fatal("NUL catalog name must fail closed")
	}
}

func TestFormatGrantCatalogRoleQuotesOwner(t *testing.T) {
	got, err := formatGrantCatalogRole("Order", "redgres_console")
	if err != nil {
		t.Fatal(err)
	}
	if got != `GRANT "Order" TO "redgres_console" WITH INHERIT TRUE, SET TRUE` {
		t.Fatalf("sql = %s", got)
	}
	if _, err := formatGrantCatalogRole("", "redgres_console"); err == nil {
		t.Fatal("empty catalog owner must fail closed")
	}
}

func TestPoolCatalogDuplicateNilPoolIsUnavailable(t *testing.T) {
	var c PoolCatalog
	if _, err := c.OwnershipSnapshot(context.Background(), "project_a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("snapshot: %v", err)
	}
	if err := c.TerminateSessions(context.Background(), "project_a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("terminate: %v", err)
	}
	if err := c.CreateDatabaseTemplate(context.Background(), "project_a_copy", "project_a", "app_project_a_copy"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("template: %v", err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
