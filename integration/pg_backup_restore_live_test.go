package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/backup"
	"github.com/SSujitX/redgres/internal/config"
)

const (
	bkDB          = "backup_live"
	bkOwner       = "app_backup_live"
	bkTargetName  = "redgres-bk-target"
	bkTargetPort  = "55436"
	bkSetID       = "feedface0123456789abcdef01234567"
)

func pgImageDigest(t *testing.T) string {
	t.Helper()
	switch livePostgresExpectedMajor(t) {
	case 18:
		return "postgres:18.6@sha256:1957b2ff3137e4ef7f3bc813e74fff50b1e1ffddc85c8b9d6f14ade972be8687"
	case 17:
		return "postgres:17.11@sha256:0b657ff48d7f76a1e907f381b1693eb4f2bf54c1d2df4feb6743d7dc601768dd"
	default:
		t.Fatalf("unsupported expected major %d", livePostgresExpectedMajor(t))
		return ""
	}
}

func dockerRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v: %v: %s", args, err, out)
	}
	return string(out)
}

func waitPGReady(t *testing.T, name string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("docker", "exec", name, "pg_isready", "-U", "postgres").CombinedOutput()
		if err == nil && strings.Contains(string(out), "accepting connections") {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("container %s not ready", name)
}

// TestLiveBackupRestoreDropGate performs a real PostgreSQL logical backup of
// a seeded project database (pg_dump via a throwaway pinned container),
// restores it into an isolated fresh PostgreSQL container (pg_restore -C),
// verifies schema + data, then builds the OPS-004/PG-011 DROP backup-gate
// manifest from the REAL artifact checksum/size and the REAL isolated-restore
// evidence so DELETE /api/v1/postgres/databases/{db} succeeds end-to-end.
func TestLiveBackupRestoreDropGate(t *testing.T) {
	clearInheritedRedgresEnv(t)
	pgHost, pgPort, pgPassFile, pgOK := livePostgresEnv(t)
	_, redisOK := liveRedisEnv(t)
	if !pgOK || !redisOK {
		t.Skip(skipLiveEnv)
	}
	provisionVault(t, pgHost, pgPort, pgPassFile)
	seedClean(t, pgHost, pgPort, pgPassFile)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Source database with schema + data via the real service.
	svc, host, port := openLivePostgresService(t)
	created, err := svc.Create(ctx, bkDB, bkOwner)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	srcConn := livePGConn(t, host, port, bkDB, bkOwner, created.Password)
	if _, err := srcConn.Exec(ctx, "CREATE TABLE public.items (id integer PRIMARY KEY, name text NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := srcConn.Exec(ctx, "INSERT INTO public.items (id, name) VALUES (1,'a'),(2,'b'),(3,'c')"); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	image := pgImageDigest(t)
	dumpDir := t.TempDir()
	dumpMount := dumpDir + ":/dump"

	// Isolated restore target (fresh container, same pinned image).
	dockerRun(t, "rm", "-f", bkTargetName)
	dockerRun(t, "run", "-d", "--name", bkTargetName, "-e", "POSTGRES_PASSWORD=redgres-ci",
		"-p", "127.0.0.1:"+bkTargetPort+":5432", image)
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", bkTargetName).Run() })
	waitPGReady(t, bkTargetName)

	// Logical dump of the source via a throwaway pinned container.
	dockerRun(t, "run", "--rm", "-e", "PGPASSWORD=redgres-ci", "-v", dumpMount, image,
		"pg_dump", "-h", "host.docker.internal", "-p", port, "-U", "postgres", "-Fc",
		"-f", "/dump/backup.dump", bkDB)
	dumpBytes, err := os.ReadFile(filepath.Join(dumpDir, "backup.dump"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dumpBytes) == 0 {
		t.Fatal("empty dump")
	}
	sum := sha256.Sum256(dumpBytes)

	// Restore into the isolated target and verify schema + data. The owner
	// role must exist on the isolated host first (a real DR operator creates
	// project roles before restore).
	dockerRun(t, "exec", bkTargetName, "psql", "-U", "postgres", "-c", "CREATE ROLE "+bkOwner+" LOGIN")
	dockerRun(t, "run", "--rm", "-e", "PGPASSWORD=redgres-ci", "-v", dumpMount, image,
		"pg_restore", "-h", "host.docker.internal", "-p", bkTargetPort, "-U", "postgres",
		"-d", "postgres", "-C", "/dump/backup.dump")
	targetConn := livePGConn(t, "127.0.0.1", bkTargetPort, bkDB, "postgres", livePGPassword(t, pgPassFile))
	var cnt int
	if err := targetConn.QueryRow(ctx, "SELECT count(*)::int FROM public.items").Scan(&cnt); err != nil {
		t.Fatalf("restored row count: %v", err)
	}
	if cnt != 3 {
		t.Fatalf("restored rows = %d want 3", cnt)
	}
	var col string
	if err := targetConn.QueryRow(ctx, "SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name='items' ORDER BY ordinal_position LIMIT 1").Scan(&col); err != nil || col != "id" {
		t.Fatalf("restored schema column = %q err %v", col, err)
	}

	// Copy the real artifact into the backup catalog jail.
	catalogDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(catalogDir, "backup.dump"), dumpBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	// Live cluster identity for the gate.
	superConn := livePGConn(t, host, port, "postgres", "postgres", livePGPassword(t, pgPassFile))
	var sysID string
	if err := superConn.QueryRow(ctx, "SELECT system_identifier::text FROM pg_control_system()").Scan(&sysID); err != nil {
		t.Fatalf("system_identifier: %v", err)
	}

	// Manifest with the REAL artifact checksum/size and REAL restore evidence.
	now := time.Now().UTC()
	m := backup.Manifest{
		SchemaVersion: backup.SchemaVersion,
		BackupSetID:   bkSetID,
		CompletedAt:   now.Add(-time.Minute),
		Cluster:       backup.ClusterIdentity{SystemIdentifier: sysID},
		Artifacts: []backup.Artifact{{
			Kind:      backup.ArtifactKindPostgresDatabase,
			Name:      bkDB,
			SHA256:    hex.EncodeToString(sum[:]),
			SizeBytes: int64(len(dumpBytes)),
			Path:      "backup.dump",
		}},
		OffHost: backup.OffHost{Completed: true, CopiedAt: now},
		Restore: backup.RestoreEvidence{
			Isolated:    true,
			Outcome:     backup.RestoreOutcomeSucceeded,
			BackupSetID: bkSetID,
			CompletedAt: now.Add(-time.Minute),
		},
		Redgres: backup.RedgresIdentity{Version: "0.0.0-live-test", CompatibilityPolicyRevision: "1"},
	}
	manifestBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalogDir, "current.json"), manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	// The DROP gate passes over HTTP with the real-artifact manifest.
	h, cookie, csrf, _, _, _ := buildLiveHTTPServer(t, func(c *config.Config) {
		c.BackupCatalogDir = catalogDir
	})
	rec := liveAuthed(t, h, http.MethodDelete, "/api/v1/postgres/databases/"+bkDB, cookie, csrf,
		`{"database_confirmation":"`+bkDB+`","owner_password":"`+liveOwnerPassword+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("drop status %d: %s", rec.Code, rec.Body.String())
	}

	// Database gone and vault row removed.
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/postgres/databases", cookie, csrf, "")
	if strings.Contains(rec.Body.String(), bkDB) {
		t.Fatal("dropped database still listed")
	}
	vconn := livePGConn(t, host, port, "database_console_vault", "postgres", livePGPassword(t, pgPassFile))
	var vaultCount int
	if err := vconn.QueryRow(ctx, "SELECT count(*)::int FROM public.project_credentials WHERE role_name = '" + bkOwner + "'").Scan(&vaultCount); err != nil {
		t.Fatalf("vault check: %v", err)
	}
	if vaultCount != 0 {
		t.Fatalf("vault row count = %d want 0", vaultCount)
	}
}
