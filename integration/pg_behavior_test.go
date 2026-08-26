package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/jackc/pgx/v5"
)

// testVaultSecret is a disposable, non-secret seed for the in-process Fernet
// vault key used by this disposable test matrix only.
const testVaultSecret = "redgres-live-matrix-vault-secret-0"

const (
	liveDB1    = "project_live"
	liveOwner1 = "app_project_live"
	liveDB2    = "project_live_copy"
	liveOwner2 = "app_project_live_copy"
)

func livePGDSN(host, port, db, user, password string) string {
	return "host=" + host + " port=" + port + " user=" + user + " dbname=" + db +
		" password=" + strings.TrimSpace(password) + " sslmode=prefer connect_timeout=10"
}

func livePGConn(t *testing.T, host, port, db, user, password string) *pgx.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, livePGDSN(host, port, db, user, password))
	if err != nil {
		t.Fatalf("connect %s as %s: %v", db, user, err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func livePGPassword(t *testing.T, passwordFile string) string {
	t.Helper()
	raw, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

// provisionVault creates the PostgreSQL vault database and table that
// postgresadmin's credential SQL expects (legacy database_console_vault
// layout). Idempotent for repeated runs against the same container.
func provisionVault(t *testing.T, host, port, passwordFile string) {
	t.Helper()
	password := livePGPassword(t, passwordFile)
	conn := livePGConn(t, host, port, "postgres", "postgres", password)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'database_console_vault')").Scan(&exists); err != nil {
		t.Fatalf("check vault database: %v", err)
	}
	if !exists {
		if _, err := conn.Exec(ctx, "CREATE DATABASE database_console_vault"); err != nil {
			t.Fatalf("create vault database: %v", err)
		}
	}
	vconn := livePGConn(t, host, port, "database_console_vault", "postgres", password)
	vctx, vcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer vcancel()
	if _, err := vconn.Exec(vctx, "CREATE TABLE IF NOT EXISTS public.project_credentials (role_name text PRIMARY KEY, encrypted_password text NOT NULL, updated_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		t.Fatalf("create vault table: %v", err)
	}
}

// seedClean removes leftovers from a previous failed run on the same container.
func seedClean(t *testing.T, host, port, passwordFile string) {
	t.Helper()
	password := livePGPassword(t, passwordFile)
	conn := livePGConn(t, host, port, "postgres", "postgres", password)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	term := "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname IN ('" + liveDB1 + "','" + liveDB2 + "') AND pid <> pg_backend_pid()"
	if _, err := conn.Exec(ctx, term); err != nil {
		t.Fatalf("terminate leftovers: %v", err)
	}
	for _, stmt := range []string{
		"DROP DATABASE IF EXISTS " + liveDB1 + " WITH (FORCE)",
		"DROP DATABASE IF EXISTS " + liveDB2 + " WITH (FORCE)",
		"DROP ROLE IF EXISTS " + liveOwner1,
		"DROP ROLE IF EXISTS " + liveOwner2,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("clean %q: %v", stmt, err)
		}
	}
}

func openLivePostgresService(t *testing.T) (*postgresadmin.Service, string, string) {
	t.Helper()
	clearInheritedRedgresEnv(t)
	host, port, passwordFile, ok := livePostgresEnv(t)
	if !ok {
		t.Skip(skipLiveEnv)
	}
	provisionVault(t, host, port, passwordFile)
	seedClean(t, host, port, passwordFile)
	secretFile := filepath.Join(t.TempDir(), "vault-secret")
	if err := os.WriteFile(secretFile, []byte(testVaultSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Environment:           config.EnvironmentDevelopment,
		PostgresHost:          host,
		PostgresPort:          port,
		PostgresDatabase:      "postgres",
		PostgresUser:          "postgres",
		PostgresPasswordFile:  passwordFile,
		PostgresSSLMode:       "prefer",
		PostgresExpectedMajor: livePostgresExpectedMajor(t),
		LegacyVaultSecretFile: secretFile,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	svc, closer, err := postgresadmin.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(closer)
	return svc, host, port
}

func containsDB(listed postgresadmin.ListResult, name string) bool {
	for _, item := range listed.Databases {
		if item.Name == name {
			return true
		}
	}
	return false
}

// TestLivePostgresServiceBehavior exercises the full project-database
// lifecycle (PG-003 create, PG-006 rotate, PG-007 tables/rows, PG-008 row
// delete, PG-009 truncate, PG-010 duplicate, PG-011 drop, PG-012 security
// overview) against a real PostgreSQL server with a provisioned vault
// database. The vault key is derived in-process from a disposable seed.
func TestLivePostgresServiceBehavior(t *testing.T) {
	svc, host, port := openLivePostgresService(t)
	password := livePGPassword(t, os.Getenv("REDGRES_TEST_POSTGRES_PASSWORD_FILE"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// --- PG-003 create ---
	created, err := svc.Create(ctx, liveDB1, liveOwner1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Database != liveDB1 || created.Owner != liveOwner1 || created.Password == "" {
		t.Fatalf("created = %+v", created)
	}
	listed, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !containsDB(listed, liveDB1) {
		t.Fatalf("List missing %s", liveDB1)
	}
	det, err := svc.Details(ctx, liveDB1)
	if err != nil {
		t.Fatalf("Details: %v", err)
	}
	if det.Owner != liveOwner1 {
		t.Fatalf("details owner = %q", det.Owner)
	}
	// Round-trip as the new owner with the generated password.
	userConn := livePGConn(t, host, port, liveDB1, liveOwner1, created.Password)
	var rolCreatedb, rolSuper int
	var rolConnLimit int
	if err := userConn.QueryRow(ctx, "SELECT rolcreatedb::int, rolsuper::int, rolconnlimit FROM pg_roles WHERE rolname = current_user").Scan(&rolCreatedb, &rolSuper, &rolConnLimit); err != nil {
		t.Fatalf("role check: %v", err)
	}
	if rolCreatedb != 0 || rolSuper != 0 {
		t.Fatalf("owner privileges createdb=%d super=%d", rolCreatedb, rolSuper)
	}
	if rolConnLimit != 20 {
		t.Fatalf("rolconnlimit = %d want 20", rolConnLimit)
	}
	var pubConnect bool
	if err := userConn.QueryRow(ctx, "SELECT has_database_privilege('public', current_database(), 'CONNECT')").Scan(&pubConnect); err != nil {
		t.Fatalf("public connect check: %v", err)
	}
	if pubConnect {
		t.Fatal("PUBLIC can still CONNECT to the project database")
	}
	// PG-012: security overview includes the project database, un-protected,
	// no PUBLIC CONNECT, and the vault status is no longer unavailable.
	ov, err := svc.SecurityOverview(ctx)
	if err != nil {
		t.Fatalf("SecurityOverview: %v", err)
	}
	if ov.SavedCredential.Status == "" {
		t.Fatal("empty vault status")
	}
	found := false
	for _, db := range ov.Databases {
		if db.Name == liveDB1 {
			found = true
			if db.Protected {
				t.Fatal("project database reported protected")
			}
			if db.PublicCanConnect {
				t.Fatal("project database reports PUBLIC CONNECT")
			}
		}
	}
	if !found {
		t.Fatalf("overview missing %s", liveDB1)
	}

	// --- PG-006 rotate ---
	rotated, err := svc.Rotate(ctx, liveDB1)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if rotated.Password == "" || rotated.Password == created.Password {
		t.Fatal("rotate did not issue a new password")
	}
	oldCtx, oldCancel := context.WithTimeout(context.Background(), 20*time.Second)
	oldConn, oldErr := pgx.Connect(oldCtx, livePGDSN(host, port, liveDB1, liveOwner1, created.Password))
	oldCancel()
	if oldErr == nil {
		_ = oldConn.Close(context.Background())
		t.Fatal("old password still accepted after rotate")
	}
	newConn := livePGConn(t, host, port, liveDB1, liveOwner1, rotated.Password)
	_ = newConn

	// --- PG-007 tables/rows, PG-008 row delete, PG-009 truncate ---
	appConn := livePGConn(t, host, port, liveDB1, liveOwner1, rotated.Password)
	if _, err := appConn.Exec(ctx, "CREATE TABLE public.items (id integer PRIMARY KEY, name text NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := appConn.Exec(ctx, "INSERT INTO public.items (id, name) VALUES (1,'a'),(2,'b'),(3,'c')"); err != nil {
		t.Fatalf("insert rows: %v", err)
	}
	tbls, err := svc.Tables(ctx, liveDB1)
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	hasItems := false
	for _, tb := range tbls.Tables {
		if tb.Schema == "public" && tb.Name == "items" {
			hasItems = true
		}
	}
	if !hasItems {
		t.Fatalf("Tables missing items: %+v", tbls)
	}
	pk, err := svc.PrimaryKey(ctx, liveDB1, "public", "items")
	if err != nil {
		t.Fatalf("PrimaryKey: %v", err)
	}
	if len(pk) != 1 || pk[0] != "id" {
		t.Fatalf("primary key = %v", pk)
	}
	page, err := svc.Rows(ctx, liveDB1, "public", "items", "", 0, 10)
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(page.Rows) != 3 {
		t.Fatalf("rows = %d want 3", len(page.Rows))
	}
	deleted, err := svc.DeleteRows(ctx, liveDB1, "public", "items", []any{1, 2})
	if err != nil {
		t.Fatalf("DeleteRows: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d want 2", deleted)
	}
	page2, err := svc.Rows(ctx, liveDB1, "public", "items", "", 0, 10)
	if err != nil {
		t.Fatalf("Rows after delete: %v", err)
	}
	if len(page2.Rows) != 1 {
		t.Fatalf("rows after delete = %d want 1", len(page2.Rows))
	}
	if _, err := svc.Truncate(ctx, liveDB1); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	page3, err := svc.Rows(ctx, liveDB1, "public", "items", "", 0, 10)
	if err != nil {
		t.Fatalf("Rows after truncate: %v", err)
	}
	if len(page3.Rows) != 0 {
		t.Fatalf("rows after truncate = %d want 0", len(page3.Rows))
	}

	// --- PG-010 duplicate ---
	dup, err := svc.Duplicate(ctx, liveDB1, liveDB2, liveOwner2)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.Database != liveDB2 || dup.Owner != liveOwner2 || dup.Password == "" {
		t.Fatalf("duplicate = %+v", dup)
	}
	dupConn := livePGConn(t, host, port, liveDB2, liveOwner2, dup.Password)
	var cnt int
	if err := dupConn.QueryRow(ctx, "SELECT count(*)::int FROM public.items").Scan(&cnt); err != nil {
		t.Fatalf("duplicate items count: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("duplicate items = %d want 0 (source truncated before copy)", cnt)
	}

	// --- PG-011 drop ---
	if _, err := svc.Drop(ctx, liveDB2); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	listed2, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List after drop: %v", err)
	}
	if containsDB(listed2, liveDB2) {
		t.Fatalf("dropped database still listed")
	}
	superConn := livePGConn(t, host, port, "postgres", "postgres", password)
	var roleExists bool
	if err := superConn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '" + liveOwner2 + "')").Scan(&roleExists); err != nil {
		t.Fatalf("role existence check: %v", err)
	}
	if roleExists {
		t.Fatal("dropped owner role still exists")
	}

	// --- guards ---
	// Protected names are indistinguishable from missing on management ops
	// (HTTP 404 contract), so Drop(postgres) must be ErrNotFound.
	if _, err := svc.Drop(ctx, "postgres"); !errors.Is(err, postgresadmin.ErrNotFound) {
		t.Fatalf("Drop(postgres) = %v want ErrNotFound", err)
	}
	if _, err := svc.Create(ctx, "postgres", "app_x"); !errors.Is(err, postgresadmin.ErrProtected) {
		t.Fatalf("Create(postgres) = %v want ErrProtected", err)
	}
	if _, err := svc.Duplicate(ctx, liveDB1, "template0", "app_x"); !errors.Is(err, postgresadmin.ErrProtected) {
		t.Fatalf("Duplicate(target template0) = %v want ErrProtected", err)
	}
}
