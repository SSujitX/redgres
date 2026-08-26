package integration

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveCreateCompensation verifies PG-003 failure compensation against a
// real PostgreSQL: when the vault credential INSERT fails after the role and
// database were created, compensation removes exactly those created resources
// (the role is dropped once it owns zero databases, the database is dropped,
// and no vault row remains).
func TestLiveCreateCompensation(t *testing.T) {
	clearInheritedRedgresEnv(t)
	svc, host, port := openLivePostgresService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	password := livePGPassword(t, os.Getenv("REDGRES_TEST_POSTGRES_PASSWORD_FILE"))
	superConn := livePGConn(t, host, port, "postgres", "postgres", password)
	vaultConn := livePGConn(t, host, port, "database_console_vault", "postgres", password)

	// Break the vault so InsertCredential fails after role+db creation.
	if _, err := vaultConn.Exec(ctx, "DROP TABLE public.project_credentials"); err != nil {
		t.Fatalf("drop vault table: %v", err)
	}
	t.Cleanup(func() {
		// Restore the vault table for later tests in the same package run and
		// remove any leftover partial create.
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		vconn := livePGConn(t, host, port, "database_console_vault", "postgres", password)
		_, _ = vconn.Exec(cleanCtx, "CREATE TABLE IF NOT EXISTS public.project_credentials (role_name text PRIMARY KEY, encrypted_password text NOT NULL, updated_at timestamptz NOT NULL DEFAULT now())")
		_, _ = svc.Drop(cleanCtx, "comp_live")
	})

	if _, err := svc.Create(ctx, "comp_live", "app_comp_live"); err == nil {
		t.Fatal("Create must fail when the vault INSERT fails")
	}

	var roleExists bool
	if err := superConn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_comp_live')").Scan(&roleExists); err != nil {
		t.Fatalf("role check: %v", err)
	}
	if roleExists {
		t.Fatal("compensated role still exists")
	}
	var dbExists bool
	if err := superConn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'comp_live')").Scan(&dbExists); err != nil {
		t.Fatalf("database check: %v", err)
	}
	if dbExists {
		t.Fatal("compensated database still exists")
	}
}
