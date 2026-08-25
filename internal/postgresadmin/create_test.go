package postgresadmin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/secrets"
)

func createService(t *testing.T, cat *MemoryCatalog, cfg config.Config, vaultKey string) *Service {
	t.Helper()
	if cfg.PostgresDatabase == "" {
		cfg.PostgresDatabase = "postgres"
	}
	if cfg.PostgresUser == "" {
		cfg.PostgresUser = "redgres_console"
	}
	return NewServiceWithVaultKey(cat, NewPolicy(cfg), vaultKey)
}

func createVaultKey(t *testing.T) string {
	t.Helper()
	fx := loadPython49(t)
	return secrets.DeriveVaultKey(fx.SessionSecret)
}

func TestCreateRoleSQLShape(t *testing.T) {
	got, err := formatCreateRole("app_project_a", "secret'pass")
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE ROLE "app_project_a" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION CONNECTION LIMIT 20 PASSWORD 'secret''pass'`
	if got != want {
		t.Fatalf("sql = %s", got)
	}
	lower := strings.ToLower(got)
	if strings.Contains(lower, "pgbouncer") {
		t.Fatal("must not create a pgbouncer role")
	}
	if strings.Contains(got, "CONNECTION LIMIT 5") {
		t.Fatal("connection limit must be constant 20")
	}
}

func TestCreateSQLHasNoEnsureVaultOrUpsert(t *testing.T) {
	blob := strings.ToLower(insertCredentialSQL)
	if strings.Contains(blob, "on conflict") {
		t.Fatal("create must not upsert")
	}
	if strings.Contains(blob, "create database") || strings.Contains(blob, "create table") {
		t.Fatal("must not ensure_vault")
	}
	if insertCredentialSQL != `INSERT INTO public.project_credentials (role_name, encrypted_password, updated_at) VALUES ($1, $2, now())` {
		t.Fatalf("sql = %s", insertCredentialSQL)
	}
	if strings.Contains(strings.ToLower(deleteCredentialSQL), "encrypted_password") {
		t.Fatal("compensation delete must not select ciphertext")
	}
}

func TestQuoteStringLiteralDoublesQuotes(t *testing.T) {
	if got := quoteStringLiteral(`a'b`); got != `'a''b'` {
		t.Fatalf("got %s", got)
	}
}

func TestServiceCreateProtectedNoDDL(t *testing.T) {
	cat := &MemoryCatalog{}
	svc := createService(t, cat, config.Config{}, createVaultKey(t))
	for _, tc := range []struct{ database, owner string }{
		{"postgres", "app_project_a"},
		{"template0", "app_project_a"},
		{"template1", "app_project_a"},
		{"database_console_vault", "app_project_a"},
		{"project_a", "postgres"},
		{"project_a", "pg_signal_backend"},
		{"project_a", "redgres_console"},
	} {
		_, err := svc.Create(context.Background(), tc.database, tc.owner)
		if !errors.Is(err, ErrProtected) {
			t.Fatalf("%s/%s: %v", tc.database, tc.owner, err)
		}
	}
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseCalls != 0 || cat.InsertCalls != 0 {
		t.Fatalf("protected must not DDL: roles=%d dbs=%d inserts=%d", cat.CreateRoleCalls, cat.CreateDatabaseCalls, cat.InsertCalls)
	}
}

func TestServiceCreateConflictDatabase(t *testing.T) {
	cat := &MemoryCatalog{Rows: []CatalogRow{projectRow("project_a", "app_other")}}
	svc := createService(t, cat, config.Config{}, createVaultKey(t))
	_, err := svc.Create(context.Background(), "project_a", "app_project_a")
	var conflict Conflict
	if !errors.As(err, &conflict) || conflict.Field != conflictFieldDatabase {
		t.Fatalf("err = %v", err)
	}
	if cat.CreateRoleCalls != 0 {
		t.Fatal("conflict must not DDL")
	}
}

func TestServiceCreateConflictRole(t *testing.T) {
	cat := &MemoryCatalog{ExistingRoles: []string{"app_project_a"}}
	svc := createService(t, cat, config.Config{}, createVaultKey(t))
	_, err := svc.Create(context.Background(), "project_a", "app_project_a")
	var conflict Conflict
	if !errors.As(err, &conflict) || conflict.Field != conflictFieldOwner {
		t.Fatalf("err = %v", err)
	}
	if cat.CreateRoleCalls != 0 {
		t.Fatal("conflict must not DDL")
	}
}

func TestServiceCreateMissingVaultKeyNoDDL(t *testing.T) {
	cat := &MemoryCatalog{}
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres", PostgresUser: "redgres_console"}))
	_, err := svc.Create(context.Background(), "project_a", "app_project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if cat.CreateRoleCalls != 0 || cat.CreateDatabaseCalls != 0 {
		t.Fatal("missing vault key must not DDL")
	}
}

func TestServiceCreateGrantSkippedForPostgresAdmin(t *testing.T) {
	cat := &MemoryCatalog{}
	svc := createService(t, cat, config.Config{PostgresUser: "postgres"}, createVaultKey(t))
	got, err := svc.Create(context.Background(), "project_a", "app_project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Database != "project_a" || got.Owner != "app_project_a" || got.Password == "" {
		t.Fatalf("got = %#v", got)
	}
	if cat.GrantCalls != 0 {
		t.Fatal("GRANT SET must skip when admin is postgres")
	}
	if !strings.Contains(cat.LastCreateRoleSQL, `CREATE ROLE "app_project_a"`) {
		t.Fatalf("sql = %s", cat.LastCreateRoleSQL)
	}
	if strings.Contains(strings.ToLower(cat.LastCreateRoleSQL), "pgbouncer") {
		t.Fatal("must not create a pgbouncer role")
	}
}

func TestServiceCreateGrantThenDatabaseFailDropsRoleOnly(t *testing.T) {
	cat := &MemoryCatalog{CreateDatabaseErr: ErrUnavailable}
	svc := createService(t, cat, config.Config{}, createVaultKey(t))
	_, err := svc.Create(context.Background(), "project_a", "app_project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if cat.CreateRoleCalls != 1 {
		t.Fatalf("CreateRoleCalls = %d", cat.CreateRoleCalls)
	}
	if cat.GrantCalls != 1 {
		t.Fatalf("GrantCalls = %d", cat.GrantCalls)
	}
	if cat.DropRoleCalls != 1 || cat.DroppedRoles[0] != "app_project_a" {
		t.Fatalf("dropped roles = %#v", cat.DroppedRoles)
	}
	if cat.DropDatabaseCalls != 0 {
		t.Fatalf("must not drop database: %d", cat.DropDatabaseCalls)
	}
	if cat.DeleteCredentialCalls != 0 {
		t.Fatal("vault was not inserted")
	}
}

func TestServiceCreateVaultInsertFailureCompensatesDropDBAndRole(t *testing.T) {
	canary := "postgresql://canary-token:secret@10.0.0.1/db"
	cat := &MemoryCatalog{InsertCredentialErr: errors.New(canary)}
	svc := createService(t, cat, config.Config{}, createVaultKey(t))
	_, err := svc.Create(context.Background(), "project_a", "app_project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if err != nil && (strings.Contains(err.Error(), "canary-token") || strings.Contains(err.Error(), canary)) {
		t.Fatalf("leaked canary: %v", err)
	}
	if cat.CreateDatabaseCalls != 1 || cat.LockConnectCalls != 1 {
		t.Fatalf("db/lock calls = %d/%d", cat.CreateDatabaseCalls, cat.LockConnectCalls)
	}
	if cat.DropDatabaseCalls != 1 || cat.DroppedDatabases[0] != "project_a" {
		t.Fatalf("dropped dbs = %#v", cat.DroppedDatabases)
	}
	if cat.DropRoleCalls != 1 || cat.DroppedRoles[0] != "app_project_a" {
		t.Fatalf("dropped roles = %#v", cat.DroppedRoles)
	}
}

func TestServiceCreateGrantFailureDropsRole(t *testing.T) {
	cat := &MemoryCatalog{GrantSetRoleErr: ErrUnavailable}
	svc := createService(t, cat, config.Config{}, createVaultKey(t))
	_, err := svc.Create(context.Background(), "project_a", "app_project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if cat.DropRoleCalls != 1 {
		t.Fatalf("DropRoleCalls = %d", cat.DropRoleCalls)
	}
	if cat.CreateDatabaseCalls != 0 {
		t.Fatal("must not CREATE DATABASE after GRANT failure")
	}
}

func TestServiceCreateSuccessEncryptsVault(t *testing.T) {
	cat := &MemoryCatalog{}
	key := createVaultKey(t)
	svc := createService(t, cat, config.Config{}, key)
	got, err := svc.Create(context.Background(), "project_a", "app_project_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Password) != 32 {
		t.Fatalf("password length = %d", len(got.Password))
	}
	if cat.InsertCalls != 1 || len(cat.InsertedVault) != 1 {
		t.Fatalf("inserts = %#v", cat.InsertedVault)
	}
	plain, err := secrets.Decrypt(key, cat.Ciphertexts["app_project_a"])
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != got.Password {
		t.Fatal("vault token must decrypt to generated password")
	}
	if cat.LastGrantSQL != `GRANT "app_project_a" TO "redgres_console" WITH INHERIT TRUE, SET TRUE` {
		t.Fatalf("grant = %s", cat.LastGrantSQL)
	}
}

func TestServiceCreateExistsCanaryIsUnavailable(t *testing.T) {
	canary := "postgresql://canary-secret@10.0.0.1/db"
	cat := &MemoryCatalog{ExistsErr: errors.New(canary)}
	svc := createService(t, cat, config.Config{}, createVaultKey(t))
	_, err := svc.Create(context.Background(), "project_a", "app_project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "canary-secret") {
		t.Fatalf("leaked canary: %v", err)
	}
	if cat.CreateRoleCalls != 0 {
		t.Fatal("exists failure must not DDL")
	}
}

func TestPoolCatalogCreateNilPoolIsUnavailable(t *testing.T) {
	var c PoolCatalog
	if _, err := c.DatabaseExists(context.Background(), "project_a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("exists: %v", err)
	}
	if err := c.CreateRole(context.Background(), "app_project_a", "secret"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("create role: %v", err)
	}
}
