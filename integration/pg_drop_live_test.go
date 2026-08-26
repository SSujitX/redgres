package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/backup"
	"github.com/SSujitX/redgres/internal/config"
)

const (
	dropLiveDB    = "drop_live"
	dropLiveOwner = "app_drop_live"
	dropSetID     = "0123456789abcdef0123456789abcdef"
)

func writeDropManifest(t *testing.T, catalogDir, systemID, dbName string, digestHex string, size int64) {
	t.Helper()
	now := time.Now().UTC()
	m := backup.Manifest{
		SchemaVersion: backup.SchemaVersion,
		BackupSetID:   dropSetID,
		CompletedAt:   now.Add(-time.Minute),
		Cluster:       backup.ClusterIdentity{SystemIdentifier: systemID},
		Artifacts: []backup.Artifact{{
			Kind:      backup.ArtifactKindPostgresDatabase,
			Name:      dbName,
			SHA256:    digestHex,
			SizeBytes: size,
			Path:      "backup.dump",
		}},
		OffHost: backup.OffHost{Completed: true, CopiedAt: now},
		Restore: backup.RestoreEvidence{
			Isolated:    true,
			Outcome:     backup.RestoreOutcomeSucceeded,
			BackupSetID: dropSetID,
			CompletedAt: now.Add(-time.Minute),
		},
		Redgres: backup.RedgresIdentity{Version: "0.0.0-live-test", CompatibilityPolicyRevision: "1"},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalogDir, "current.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLiveHTTPDropWithBackupCatalog verifies the PG-011 DROP backup-gate over
// HTTP against a real PostgreSQL cluster: a manifest whose cluster identity
// does not match is denied 403 with the evaluator reason, and a manifest
// matching the live pg_control_system() identity plus fresh off-host/restore
// evidence allows the drop, which removes the database, the owner role, and
// the vault row.
func TestLiveHTTPDropWithBackupCatalog(t *testing.T) {
	clearInheritedRedgresEnv(t)
	pgHost, pgPort, pgPassFile, pgOK := livePostgresEnv(t)
	_, redisOK := liveRedisEnv(t)
	if !pgOK || !redisOK {
		t.Skip(skipLiveEnv)
	}
	provisionVault(t, pgHost, pgPort, pgPassFile)
	seedClean(t, pgHost, pgPort, pgPassFile)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	superConn := livePGConn(t, pgHost, pgPort, "postgres", "postgres", livePGPassword(t, pgPassFile))
	var sysID string
	if err := superConn.QueryRow(ctx, "SELECT system_identifier::text FROM pg_control_system()").Scan(&sysID); err != nil {
		t.Fatalf("system_identifier: %v", err)
	}

	artifactBytes := bytes.Repeat([]byte{0xAB}, 64)
	digest := sha256.Sum256(artifactBytes)
	catalogDir := filepath.Join(t.TempDir(), "backup-jail")
	if err := os.MkdirAll(catalogDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalogDir, "backup.dump"), artifactBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	h, cookie, csrf, _, _, _ := buildLiveHTTPServer(t, func(c *config.Config) {
		c.BackupCatalogDir = catalogDir
	})

	// Create the project database over HTTP first.
	rec := liveAuthed(t, h, http.MethodPost, "/api/v1/postgres/databases", cookie, csrf,
		`{"database":"`+dropLiveDB+`","owner":"`+dropLiveOwner+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", rec.Code, rec.Body.String())
	}

	dropReq := `{"database_confirmation":"` + dropLiveDB + `","owner_password":"` + liveOwnerPassword + `"}`
	dropPath := "/api/v1/postgres/databases/" + dropLiveDB

	// Deny: cluster identity mismatch -> 403 with the evaluator reason.
	writeDropManifest(t, catalogDir, "99999999999999999999", dropLiveDB, hex.EncodeToString(digest[:]), int64(len(artifactBytes)))
	rec = liveAuthed(t, h, http.MethodDelete, dropPath, cookie, csrf, dropReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("deny drop status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Backup cluster identity does not match") {
		t.Fatalf("deny drop reason = %s", rec.Body.String())
	}

	// Allow: matching live identity + fresh off-host/restore evidence -> 200.
	writeDropManifest(t, catalogDir, sysID, dropLiveDB, hex.EncodeToString(digest[:]), int64(len(artifactBytes)))
	rec = liveAuthed(t, h, http.MethodDelete, dropPath, cookie, csrf, dropReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("allow drop status %d: %s", rec.Code, rec.Body.String())
	}

	// The database is gone from the manageable list.
	rec = liveAuthed(t, h, http.MethodGet, "/api/v1/postgres/databases", cookie, csrf, "")
	var listed struct {
		Databases []struct {
			Name string `json:"name"`
		} `json:"databases"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("list decode: %v", err)
	}
	for _, item := range listed.Databases {
		if item.Name == dropLiveDB {
			t.Fatal("dropped database still listed")
		}
	}

	// The owner role and vault row are gone.
	var roleExists bool
	if err := superConn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '"+dropLiveOwner+"')").Scan(&roleExists); err != nil {
		t.Fatalf("role check: %v", err)
	}
	if roleExists {
		t.Fatal("dropped owner role still exists")
	}
	vconn := livePGConn(t, pgHost, pgPort, "database_console_vault", "postgres", livePGPassword(t, pgPassFile))
	var vaultCount int
	if err := vconn.QueryRow(ctx, "SELECT count(*)::int FROM public.project_credentials WHERE role_name = '"+dropLiveOwner+"'").Scan(&vaultCount); err != nil {
		t.Fatalf("vault check: %v", err)
	}
	if vaultCount != 0 {
		t.Fatalf("vault row count = %d want 0", vaultCount)
	}
}
