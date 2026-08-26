package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/audit"
	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/internal/operations"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/SSujitX/redgres/internal/secrets"
)

const (
	dupSourceDB    = "dup_live"
	dupSourceOwner = "app_dup_live"
	dupCopyDB      = "dup_live_copy"
	dupCopyOwner   = "app_dup_live_copy"
)

// TestLiveDuplicate202OverHTTP exercises the PG-010 202-queue journey over
// HTTP against real PostgreSQL and a real SQLite operations store: POST
// duplicate returns 202 queued, the operation reports queued, a worker pass
// (RunQueuedDuplicates with the real service and compensator) transitions it
// to succeeded and clones the database with a unique restricted owner + vault
// row, and the copy is reachable with the vault-decrypted password.
func TestLiveDuplicate202OverHTTP(t *testing.T) {
	h, cookie, csrf, pgHost, pgPort, dbPath := buildLiveHTTPServer(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Source database via HTTP.
	rec := liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases", cookie, csrf,
		`{"database":"`+dupSourceDB+`","owner":"`+dupSourceOwner+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create source status %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Credential struct {
			Password string `json:"password"`
		} `json:"credential"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	// Seed a table so the clone is meaningful.
	srcConn := livePGConn(t, pgHost, pgPort, dupSourceDB, dupSourceOwner, created.Credential.Password)
	if _, err := srcConn.Exec(ctx, "CREATE TABLE public.items (id integer PRIMARY KEY, name text NOT NULL)"); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if _, err := srcConn.Exec(ctx, "INSERT INTO public.items (id, name) VALUES (1,'a'),(2,'b')"); err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	// POST duplicate -> 202 queued.
	rec = liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases/"+dupSourceDB+"/duplicate", cookie, csrf,
		`{"database":"`+dupCopyDB+`","owner":"`+dupCopyOwner+`"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("duplicate status %d: %s", rec.Code, rec.Body.String())
	}
	var accepted struct {
		Operation struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"operation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Operation.ID == "" || accepted.Operation.Status != "queued" {
		t.Fatalf("accepted = %+v", accepted.Operation)
	}

	// Operation reports queued; the copy must not exist yet.
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/operations/"+accepted.Operation.ID, cookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("operation status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"queued"`) {
		t.Fatalf("operation body = %s", rec.Body.String())
	}
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/postgres/databases", cookie, csrf, "")
	if strings.Contains(rec.Body.String(), dupCopyDB) {
		t.Fatal("copy listed before the worker ran")
	}

	// Worker pass: re-open the SQLite store + a real service + audit store.
	db2, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	store := operations.NewStore(db2)

	secretFile := filepath.Join(t.TempDir(), "vault-secret-worker")
	if err := os.WriteFile(secretFile, []byte(testVaultSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	workerCfg := config.Config{
		Environment:           config.EnvironmentDevelopment,
		PostgresHost:          pgHost,
		PostgresPort:          pgPort,
		PostgresDatabase:      "postgres",
		PostgresUser:          "postgres",
		PostgresPasswordFile:  os.Getenv("REDGRES_TEST_POSTGRES_PASSWORD_FILE"),
		PostgresSSLMode:       "prefer",
		PostgresExpectedMajor: livePostgresExpectedMajor(t),
		LegacyVaultSecretFile: secretFile,
	}
	svc, closer, err := postgresadmin.Open(ctx, workerCfg)
	if err != nil {
		t.Fatalf("worker postgresadmin.Open: %v", err)
	}
	defer closer()

	auditor := audit.Store{DB: db2}
	if err := postgresadmin.RunQueuedDuplicates(ctx, store, svc, auditor); err != nil {
		t.Fatalf("RunQueuedDuplicates: %v", err)
	}

	// Operation now succeeded; copy is listed.
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/operations/"+accepted.Operation.ID, cookie, csrf, "")
	if !strings.Contains(rec.Body.String(), `"status":"succeeded"`) {
		t.Fatalf("operation after worker = %s", rec.Body.String())
	}
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/postgres/databases", cookie, csrf, "")
	if !strings.Contains(rec.Body.String(), dupCopyDB) {
		t.Fatal("copy missing after worker")
	}

	// The vault row for the copy owner decrypts (same disposable seed) to a
	// working password for the copy database.
	var token string
	vconn := livePGConn(t, pgHost, pgPort, "database_console_vault", "postgres", livePGPassword(t, os.Getenv("REDGRES_TEST_POSTGRES_PASSWORD_FILE")))
	if err := vconn.QueryRow(ctx, "SELECT encrypted_password FROM public.project_credentials WHERE role_name = '"+dupCopyOwner+"'").Scan(&token); err != nil {
		t.Fatalf("vault row: %v", err)
	}
	plain, err := secrets.Decrypt(secrets.DeriveVaultKey(testVaultSecret), token)
	if err != nil {
		t.Fatalf("decrypt copy password: %v", err)
	}
	copyConn := livePGConn(t, pgHost, pgPort, dupCopyDB, dupCopyOwner, string(plain))
	var items int
	if err := copyConn.QueryRow(ctx, "SELECT count(*)::int FROM public.items").Scan(&items); err != nil {
		t.Fatalf("copy items: %v", err)
	}
	if items != 2 {
		t.Fatalf("copy items = %d want 2", items)
	}
	var owner string
	if err := copyConn.QueryRow(ctx, "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database()").Scan(&owner); err != nil {
		t.Fatalf("copy owner: %v", err)
	}
	if owner != dupCopyOwner {
		t.Fatalf("copy owner = %q want %q", owner, dupCopyOwner)
	}

	// Audit records the duplicate success.
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/audit?limit=50", cookie, csrf, "")
	var auditBody struct {
		Events []struct {
			Action  string `json:"action"`
			Outcome string `json:"outcome"`
		} `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &auditBody); err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, ev := range auditBody.Events {
		if ev.Action == "postgres.database.duplicate" && ev.Outcome == "success" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("audit missing postgres.database.duplicate/success")
	}
}
