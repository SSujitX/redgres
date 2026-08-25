package postgresadmin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/secrets"
)

func eligibleRotateRow(name, owner string) CatalogRow {
	return CatalogRow{Name: name, Owner: owner, AllowConn: true, OwnerCanLogin: true}
}

func rotateService(t *testing.T, cat *MemoryCatalog, vaultKey string) *Service {
	t.Helper()
	return createService(t, cat, config.Config{}, vaultKey)
}

func TestAlterRolePasswordSQLQuoted(t *testing.T) {
	got, err := formatAlterRolePassword("app_project_a", "secret'pass")
	if err != nil {
		t.Fatal(err)
	}
	want := `ALTER ROLE "app_project_a" WITH PASSWORD 'secret''pass' CONNECTION LIMIT 20`
	if got != want {
		t.Fatalf("sql = %s", got)
	}
	if strings.Contains(got, "CONNECTION LIMIT 5") {
		t.Fatal("connection limit must be constant 20")
	}
}

func TestUpsertCredentialSQLHasOnConflict(t *testing.T) {
	lower := strings.ToLower(upsertCredentialSQL)
	if !strings.Contains(lower, "on conflict (role_name)") {
		t.Fatalf("upsert must ON CONFLICT: %s", upsertCredentialSQL)
	}
	if !strings.Contains(lower, "do update set encrypted_password = excluded.encrypted_password") {
		t.Fatalf("upsert must update ciphertext: %s", upsertCredentialSQL)
	}
	if strings.Contains(lower, "create database") || strings.Contains(lower, "create table") {
		t.Fatal("must not ensure_vault")
	}
	if strings.Contains(strings.ToLower(insertCredentialSQL), "on conflict") {
		t.Fatal("create INSERT must stay without ON CONFLICT")
	}
}

func TestServiceRotateEligibilityRejectsWithoutAlter(t *testing.T) {
	key := createVaultKey(t)
	cases := []struct {
		name string
		row  CatalogRow
		want error
	}{
		{name: "missing", row: CatalogRow{}, want: ErrNotFound},
		{name: "protected-db", row: eligibleRotateRow("postgres", "app_project_a"), want: ErrNotFound},
		{name: "template", row: CatalogRow{Name: "project_a", Owner: "app_project_a", AllowConn: true, IsTemplate: true, OwnerCanLogin: true}, want: ErrNotFound},
		{name: "no-connect", row: CatalogRow{Name: "project_a", Owner: "app_project_a", AllowConn: false, OwnerCanLogin: true}, want: ErrNotFound},
		{name: "empty-owner", row: CatalogRow{Name: "project_a", Owner: "", AllowConn: true, OwnerCanLogin: true}, want: ErrNotFound},
		{name: "superuser", row: CatalogRow{Name: "project_a", Owner: "app_project_a", AllowConn: true, OwnerCanLogin: true, OwnerIsSuperuser: true}, want: ErrProtected},
		{name: "non-login", row: CatalogRow{Name: "project_a", Owner: "app_project_a", AllowConn: true, OwnerCanLogin: false}, want: ErrProtected},
		{name: "owner-denied", row: eligibleRotateRow("project_a", "postgres"), want: ErrProtected},
		{name: "pg-prefix", row: eligibleRotateRow("project_a", "pg_signal_backend"), want: ErrProtected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := &MemoryCatalog{}
			if tc.row.Name != "" {
				cat.Rows = []CatalogRow{tc.row}
			}
			svc := rotateService(t, cat, key)
			db := "project_a"
			if tc.name == "protected-db" {
				db = "postgres"
			}
			got, err := svc.Rotate(context.Background(), db)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v want %v", err, tc.want)
			}
			if got.Password != "" {
				t.Fatal("must not return password")
			}
			if cat.AlterRolePasswordCalls != 0 || cat.UpsertCalls != 0 {
				t.Fatalf("must not ALTER/upsert: alter=%d upsert=%d", cat.AlterRolePasswordCalls, cat.UpsertCalls)
			}
		})
	}
}

func TestServiceRotateMissingVaultKeyNoAlter(t *testing.T) {
	cat := &MemoryCatalog{Rows: []CatalogRow{eligibleRotateRow("project_a", "app_project_a")}}
	svc := NewService(cat, NewPolicy(config.Config{PostgresDatabase: "postgres", PostgresUser: "redgres_console"}))
	_, err := svc.Rotate(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if cat.AlterRolePasswordCalls != 0 || cat.UpsertCalls != 0 {
		t.Fatal("missing vault key must not ALTER")
	}
}

func TestServiceRotateVaultProbeFailNoAlter(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:     []CatalogRow{eligibleRotateRow("project_a", "app_project_a")},
		VaultErr: ErrUnavailable,
	}
	svc := rotateService(t, cat, createVaultKey(t))
	_, err := svc.Rotate(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if cat.AlterRolePasswordCalls != 0 || cat.UpsertCalls != 0 {
		t.Fatal("vault probe fail must not ALTER")
	}
}

func TestServiceRotateLockConflictNoSecondAlter(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:         []CatalogRow{eligibleRotateRow("project_a", "app_project_a")},
		AlterStarted: make(chan struct{}, 1),
		AlterHold:    make(chan struct{}),
	}
	svc := rotateService(t, cat, createVaultKey(t))
	done := make(chan error, 1)
	go func() {
		_, err := svc.Rotate(context.Background(), "project_a")
		done <- err
	}()
	select {
	case <-cat.AlterStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first rotate did not reach ALTER")
	}
	got, err := svc.Rotate(context.Background(), "project_a")
	if !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("second err = %v", err)
	}
	if got.Password != "" {
		t.Fatal("409 must not return password")
	}
	close(cat.AlterHold)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("first rotate: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first rotate blocked")
	}
	if cat.AlterRolePasswordCalls != 1 {
		t.Fatalf("AlterRolePasswordCalls = %d", cat.AlterRolePasswordCalls)
	}
}

func TestServiceRotateVaultFailAfterAlterNoPasswordNoReAlter(t *testing.T) {
	canary := "postgresql://canary-rotate-secret@10.0.0.1/db"
	cat := &MemoryCatalog{
		Rows:                []CatalogRow{eligibleRotateRow("project_a", "app_project_a")},
		UpsertCredentialErr: errors.New(canary),
	}
	svc := rotateService(t, cat, createVaultKey(t))
	got, err := svc.Rotate(context.Background(), "project_a")
	if !errors.Is(err, ErrVaultUnsynced) {
		t.Fatalf("err = %v", err)
	}
	if err != nil && (strings.Contains(err.Error(), canary) || strings.Contains(err.Error(), "canary-rotate-secret")) {
		t.Fatalf("leaked canary: %v", err)
	}
	if got.Password != "" {
		t.Fatal("must not return password after vault fail")
	}
	if cat.AlterRolePasswordCalls != 1 {
		t.Fatalf("must not re-ALTER: %d", cat.AlterRolePasswordCalls)
	}
	if cat.UpsertCalls != vaultUpsertAttempts {
		t.Fatalf("upsert attempts = %d want %d", cat.UpsertCalls, vaultUpsertAttempts)
	}
	if cat.InsertCalls != 0 {
		t.Fatal("rotate must not use create INSERT")
	}
}

func TestServiceRotateUpsertRetriesThenSucceeds(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:            []CatalogRow{eligibleRotateRow("project_a", "app_project_a")},
		UpsertFailTimes: 2,
	}
	svc := rotateService(t, cat, createVaultKey(t))
	got, err := svc.Rotate(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Password == "" || got.Owner != "app_project_a" {
		t.Fatalf("got = %#v", got)
	}
	if cat.AlterRolePasswordCalls != 1 {
		t.Fatalf("re-ALTER: %d", cat.AlterRolePasswordCalls)
	}
	if cat.UpsertCalls != vaultUpsertAttempts {
		t.Fatalf("upsert calls = %d", cat.UpsertCalls)
	}
}

func TestServiceRotateSuccessEncryptsVaultWithoutInsert(t *testing.T) {
	cat := &MemoryCatalog{Rows: []CatalogRow{eligibleRotateRow("project_a", "app_project_a")}}
	key := createVaultKey(t)
	svc := rotateService(t, cat, key)
	got, err := svc.Rotate(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Database != "project_a" || got.Owner != "app_project_a" || len(got.Password) != 32 {
		t.Fatalf("got = %#v", got)
	}
	if cat.InsertCalls != 0 {
		t.Fatal("create INSERT must not run")
	}
	if cat.UpsertCalls != 1 {
		t.Fatalf("upsert calls = %d", cat.UpsertCalls)
	}
	if !strings.Contains(cat.LastAlterRoleSQL, `ALTER ROLE "app_project_a" WITH PASSWORD `) {
		t.Fatalf("sql = %s", cat.LastAlterRoleSQL)
	}
	if strings.Contains(cat.LastAlterRoleSQL, got.Password) && !strings.Contains(cat.LastAlterRoleSQL, quoteStringLiteral(got.Password)) {
		t.Fatal("password must be string-literal quoted")
	}
	plain, err := secrets.Decrypt(key, cat.LastUpsertToken)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != got.Password {
		t.Fatal("vault token must decrypt to generated password")
	}
}

func TestPoolCatalogRotateNilPoolIsUnavailable(t *testing.T) {
	var c PoolCatalog
	if err := c.AlterRolePassword(context.Background(), "app_project_a", "secret"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("alter: %v", err)
	}
	if err := c.UpsertCredential(context.Background(), "app_project_a", "token"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("upsert: %v", err)
	}
}
