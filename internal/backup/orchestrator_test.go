package backup_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/backup"
)

func TestOrchestratorRejectsCatalogThroughSymlinkAncestor(t *testing.T) {
	outside := t.TempDir()
	catalog := filepath.Join(outside, "catalog")
	if err := os.Mkdir(catalog, 0o700); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	link := filepath.Join(parent, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	called := false
	orchestrator, err := backup.NewOrchestrator(backup.OrchestratorAdapters{
		PostgreSQL: postgresCaptureFunc(func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			called = true
			return nil, nil
		}),
		Redis: redisCaptureFunc(func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error) { return nil, nil }),
		SQLite: sqliteCaptureFunc(func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			return nil, nil
		}),
		Config: configCaptureFunc(func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			return nil, nil
		}),
		OffHost: offHostCopyFunc(func(context.Context, backup.Snapshot) (backup.OffHost, error) {
			return backup.OffHost{}, nil
		}),
		Restore: restoreVerifyFunc(func(context.Context, backup.Snapshot) (backup.RestoreEvidence, error) {
			return backup.RestoreEvidence{}, nil
		}),
		Retention: retentionPlanFunc(func(context.Context, backup.RetentionInput) (backup.RetentionPlan, error) {
			return backup.RetentionPlan{}, nil
		}),
		Now: time.Now,
	})
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	_, err = orchestrator.Run(context.Background(), backup.BackupRequest{
		CatalogDir:       filepath.Join(link, "catalog"),
		BackupSetID:      "0123456789abcdef0123456789abcdef",
		SystemIdentifier: "7439123456789012345",
	})
	if !errors.Is(err, backup.ErrInvalidBackupRequest) {
		t.Fatalf("err = %v, want ErrInvalidBackupRequest", err)
	}
	if called {
		t.Fatal("capture adapter ran for a symlinked catalog ancestor")
	}
	entries, readErr := os.ReadDir(catalog)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("outside catalog was touched: %v", entries)
	}
}

func TestNewOrchestratorRejectsTypedNilAdapter(t *testing.T) {
	var postgres *nilPostgresCapture
	_, err := backup.NewOrchestrator(backup.OrchestratorAdapters{
		PostgreSQL: postgres,
		Redis: redisCaptureFunc(func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			return nil, nil
		}),
		SQLite: sqliteCaptureFunc(func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			return nil, nil
		}),
		Config: configCaptureFunc(func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			return nil, nil
		}),
		OffHost: offHostCopyFunc(func(context.Context, backup.Snapshot) (backup.OffHost, error) {
			return backup.OffHost{}, nil
		}),
		Restore: restoreVerifyFunc(func(context.Context, backup.Snapshot) (backup.RestoreEvidence, error) {
			return backup.RestoreEvidence{}, nil
		}),
		Retention: retentionPlanFunc(func(context.Context, backup.RetentionInput) (backup.RetentionPlan, error) {
			return backup.RetentionPlan{}, nil
		}),
		Now: time.Now,
	})
	if !errors.Is(err, backup.ErrInvalidOrchestrator) {
		t.Fatalf("err = %v, want ErrInvalidOrchestrator", err)
	}
}

func TestOrchestratorPublishesOnlyCompleteRecoverableSet(t *testing.T) {
	completedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	setID := "0123456789abcdef0123456789abcdef"
	orchestrator, err := backup.NewOrchestrator(backup.OrchestratorAdapters{
		PostgreSQL: postgresCaptureFunc(func(_ context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			writeCaptureFile(t, target.Root, "appdb.dump", "abc")
			writeCaptureFile(t, target.Root, "globals.sql", "postgres-globals")
			return []backup.ProducedArtifact{
				{Kind: backup.ArtifactKindPostgresDatabase, Name: "appdb", Path: "appdb.dump"},
				{Kind: "postgres.globals", Name: "cluster", Path: "globals.sql"},
			}, nil
		}),
		Redis: redisCaptureFunc(func(_ context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			writeCaptureFile(t, target.Root, "dump.rdb", "redis-rdb")
			writeCaptureFile(t, target.Root, "users.acl", "sanitized-acl")
			return []backup.ProducedArtifact{
				{Kind: "redis.rdb", Name: "redis", Path: "dump.rdb"},
				{Kind: "redis.acl", Name: "users", Path: "users.acl"},
			}, nil
		}),
		SQLite: sqliteCaptureFunc(func(_ context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			writeCaptureFile(t, target.Root, "redgres.db", "sqlite-online-backup")
			return []backup.ProducedArtifact{{Kind: "redgres.sqlite", Name: "control-state", Path: "redgres.db"}}, nil
		}),
		Config: configCaptureFunc(func(_ context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			writeCaptureFile(t, target.Root, "config.json", `{"checksum":"public-only"}`)
			return []backup.ProducedArtifact{{Kind: "redgres.config", Name: "configuration", Path: "config.json"}}, nil
		}),
		OffHost: offHostCopyFunc(func(_ context.Context, snapshot backup.Snapshot) (backup.OffHost, error) {
			if len(snapshot.Artifacts) != 6 {
				t.Fatalf("off-host snapshot artifacts = %d, want 6", len(snapshot.Artifacts))
			}
			return backup.OffHost{Completed: true, CopiedAt: completedAt.Add(time.Minute)}, nil
		}),
		Restore: restoreVerifyFunc(func(_ context.Context, snapshot backup.Snapshot) (backup.RestoreEvidence, error) {
			return backup.RestoreEvidence{
				Isolated:    true,
				Outcome:     backup.RestoreOutcomeSucceeded,
				BackupSetID: snapshot.BackupSetID,
				CompletedAt: completedAt.Add(2 * time.Minute),
			}, nil
		}),
		Retention: retentionPlanFunc(func(_ context.Context, _ backup.RetentionInput) (backup.RetentionPlan, error) {
			return backup.RetentionPlan{}, nil
		}),
		Now: func() time.Time { return completedAt },
	})
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	catalogDir := t.TempDir()
	result, err := orchestrator.Run(context.Background(), backup.BackupRequest{
		CatalogDir:       catalogDir,
		BackupSetID:      setID,
		SystemIdentifier: "7439123456789012345",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Manifest.BackupSetID != setID {
		t.Fatalf("result backup_set_id = %q", result.Manifest.BackupSetID)
	}

	manifest, err := backup.LoadCurrent(catalogDir)
	if err != nil {
		t.Fatalf("LoadCurrent: %v", err)
	}
	if len(manifest.Artifacts) != 6 {
		t.Fatalf("published artifacts = %d, want 6", len(manifest.Artifacts))
	}
	postgres := manifest.Artifacts[0]
	if postgres.Path != "sets/0123456789abcdef0123456789abcdef/postgres/appdb.dump" {
		t.Fatalf("PostgreSQL artifact path = %q", postgres.Path)
	}
	if postgres.SizeBytes != 3 {
		t.Fatalf("PostgreSQL artifact size = %d, want 3", postgres.SizeBytes)
	}
	if postgres.SHA256 != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("PostgreSQL artifact SHA-256 = %q", postgres.SHA256)
	}
	if !manifest.OffHost.Completed || !manifest.Restore.Isolated {
		t.Fatalf("mandatory evidence missing: off_host=%+v restore=%+v", manifest.OffHost, manifest.Restore)
	}
	if _, err := os.Stat(filepath.Join(catalogDir, filepath.FromSlash(postgres.Path))); err != nil {
		t.Fatalf("published artifact: %v", err)
	}
}

func TestOrchestratorFailureDoesNotPublishOrLeakAdapterError(t *testing.T) {
	completedAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	adapters := completeAdapters(t, completedAt)
	const canary = "restore-token-canary-71f9"
	adapters.Restore = restoreVerifyFunc(func(context.Context, backup.Snapshot) (backup.RestoreEvidence, error) {
		return backup.RestoreEvidence{}, errors.New(canary)
	})
	orchestrator, err := backup.NewOrchestrator(adapters)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	catalogDir := t.TempDir()

	_, err = orchestrator.Run(context.Background(), validBackupRequest(catalogDir, "1123456789abcdef0123456789abcdef"))
	if !errors.Is(err, backup.ErrRestoreVerificationFailed) {
		t.Fatalf("err = %v, want ErrRestoreVerificationFailed", err)
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatalf("error leaked adapter canary: %q", err)
	}
	assertCatalogEmpty(t, catalogDir)
}

func TestOrchestratorCancellationCleansDisposableRoot(t *testing.T) {
	completedAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	adapters := completeAdapters(t, completedAt)
	ctx, cancel := context.WithCancel(context.Background())
	adapters.PostgreSQL = postgresCaptureFunc(func(_ context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
		writeCaptureFile(t, target.Root, "appdb.dump", "abc")
		cancel()
		return []backup.ProducedArtifact{{Kind: backup.ArtifactKindPostgresDatabase, Name: "appdb", Path: "appdb.dump"}}, nil
	})
	orchestrator, err := backup.NewOrchestrator(adapters)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	catalogDir := t.TempDir()

	_, err = orchestrator.Run(ctx, validBackupRequest(catalogDir, "2123456789abcdef0123456789abcdef"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	assertCatalogEmpty(t, catalogDir)
}

func TestOrchestratorRechecksArtifactAfterExternalEvidence(t *testing.T) {
	completedAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	adapters := completeAdapters(t, completedAt)
	adapters.Restore = restoreVerifyFunc(func(_ context.Context, snapshot backup.Snapshot) (backup.RestoreEvidence, error) {
		path := filepath.Join(snapshot.Root, filepath.FromSlash(snapshot.Artifacts[0].Path))
		if err := os.WriteFile(path, []byte("changed-after-copy"), 0o600); err != nil {
			t.Fatal(err)
		}
		return successfulRestore(snapshot, completedAt.Add(2*time.Minute)), nil
	})
	orchestrator, err := backup.NewOrchestrator(adapters)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	catalogDir := t.TempDir()

	_, err = orchestrator.Run(context.Background(), validBackupRequest(catalogDir, "3123456789abcdef0123456789abcdef"))
	if !errors.Is(err, backup.ErrArtifactVerification) {
		t.Fatalf("err = %v, want ErrArtifactVerification", err)
	}
	assertCatalogEmpty(t, catalogDir)
}

func TestOrchestratorRejectsUnsafeRetentionPlanBeforePublication(t *testing.T) {
	completedAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	setID := "4123456789abcdef0123456789abcdef"
	adapters := completeAdapters(t, completedAt)
	adapters.Retention = retentionPlanFunc(func(context.Context, backup.RetentionInput) (backup.RetentionPlan, error) {
		return backup.RetentionPlan{RemoveSetIDs: []string{setID}}, nil
	})
	orchestrator, err := backup.NewOrchestrator(adapters)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	catalogDir := t.TempDir()

	_, err = orchestrator.Run(context.Background(), validBackupRequest(catalogDir, setID))
	if !errors.Is(err, backup.ErrRetentionPlanFailed) {
		t.Fatalf("err = %v, want ErrRetentionPlanFailed", err)
	}
	assertCatalogEmpty(t, catalogDir)
}

func TestOrchestratorRejectsArtifactOutsideCaptureJail(t *testing.T) {
	completedAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	adapters := completeAdapters(t, completedAt)
	outside := filepath.Join(t.TempDir(), "outside-canary.dump")
	const canary = "outside-artifact-canary"
	if err := os.WriteFile(outside, []byte(canary), 0o600); err != nil {
		t.Fatal(err)
	}
	adapters.PostgreSQL = postgresCaptureFunc(func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
		return []backup.ProducedArtifact{{Kind: backup.ArtifactKindPostgresDatabase, Name: "appdb", Path: outside}}, nil
	})
	orchestrator, err := backup.NewOrchestrator(adapters)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	catalogDir := t.TempDir()

	_, err = orchestrator.Run(context.Background(), validBackupRequest(catalogDir, "7123456789abcdef0123456789abcdef"))
	if !errors.Is(err, backup.ErrArtifactVerification) {
		t.Fatalf("err = %v, want ErrArtifactVerification", err)
	}
	raw, readErr := os.ReadFile(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(raw) != canary {
		t.Fatalf("outside canary changed: %q", raw)
	}
	assertCatalogEmpty(t, catalogDir)
}

func TestOrchestratorReplacesCurrentOnlyWithLaterCompleteSet(t *testing.T) {
	completedAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	orchestrator, err := backup.NewOrchestrator(completeAdapters(t, completedAt))
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	catalogDir := t.TempDir()
	first := "5123456789abcdef0123456789abcdef"
	second := "6123456789abcdef0123456789abcdef"
	if _, err := orchestrator.Run(context.Background(), validBackupRequest(catalogDir, first)); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if _, err := orchestrator.Run(context.Background(), validBackupRequest(catalogDir, second)); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	manifest, err := backup.LoadCurrent(catalogDir)
	if err != nil {
		t.Fatalf("LoadCurrent: %v", err)
	}
	if manifest.BackupSetID != second {
		t.Fatalf("current backup_set_id = %q, want %q", manifest.BackupSetID, second)
	}
	for _, setID := range []string{first, second} {
		path := filepath.Join(catalogDir, "sets", setID, "postgres", "appdb.dump")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("set %s artifact: %v", setID, err)
		}
	}
}

type postgresCaptureFunc func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error)

func (f postgresCaptureFunc) CaptureLogical(ctx context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
	return f(ctx, target)
}

type nilPostgresCapture struct{}

func (*nilPostgresCapture) CaptureLogical(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
	panic("typed-nil adapter must not be called")
}

type redisCaptureFunc func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error)

func (f redisCaptureFunc) CaptureAtomic(ctx context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
	return f(ctx, target)
}

type sqliteCaptureFunc func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error)

func (f sqliteCaptureFunc) CaptureOnline(ctx context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
	return f(ctx, target)
}

type configCaptureFunc func(context.Context, backup.CaptureTarget) ([]backup.ProducedArtifact, error)

func (f configCaptureFunc) CaptureSanitized(ctx context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
	return f(ctx, target)
}

type offHostCopyFunc func(context.Context, backup.Snapshot) (backup.OffHost, error)

func (f offHostCopyFunc) CopyEncrypted(ctx context.Context, snapshot backup.Snapshot) (backup.OffHost, error) {
	return f(ctx, snapshot)
}

type restoreVerifyFunc func(context.Context, backup.Snapshot) (backup.RestoreEvidence, error)

func (f restoreVerifyFunc) VerifyIsolated(ctx context.Context, snapshot backup.Snapshot) (backup.RestoreEvidence, error) {
	return f(ctx, snapshot)
}

type retentionPlanFunc func(context.Context, backup.RetentionInput) (backup.RetentionPlan, error)

func (f retentionPlanFunc) Plan(ctx context.Context, input backup.RetentionInput) (backup.RetentionPlan, error) {
	return f(ctx, input)
}

func writeCaptureFile(t *testing.T, root, rel, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func completeAdapters(t *testing.T, completedAt time.Time) backup.OrchestratorAdapters {
	t.Helper()
	return backup.OrchestratorAdapters{
		PostgreSQL: postgresCaptureFunc(func(_ context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			writeCaptureFile(t, target.Root, "appdb.dump", "abc")
			writeCaptureFile(t, target.Root, "globals.sql", "postgres-globals")
			return []backup.ProducedArtifact{
				{Kind: backup.ArtifactKindPostgresDatabase, Name: "appdb", Path: "appdb.dump"},
				{Kind: "postgres.globals", Name: "cluster", Path: "globals.sql"},
			}, nil
		}),
		Redis: redisCaptureFunc(func(_ context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			writeCaptureFile(t, target.Root, "dump.rdb", "redis-rdb")
			return []backup.ProducedArtifact{{Kind: "redis.rdb", Name: "redis", Path: "dump.rdb"}}, nil
		}),
		SQLite: sqliteCaptureFunc(func(_ context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			writeCaptureFile(t, target.Root, "redgres.db", "sqlite-online-backup")
			return []backup.ProducedArtifact{{Kind: "redgres.sqlite", Name: "control-state", Path: "redgres.db"}}, nil
		}),
		Config: configCaptureFunc(func(_ context.Context, target backup.CaptureTarget) ([]backup.ProducedArtifact, error) {
			writeCaptureFile(t, target.Root, "config.json", `{"checksum":"public-only"}`)
			return []backup.ProducedArtifact{{Kind: "redgres.config", Name: "configuration", Path: "config.json"}}, nil
		}),
		OffHost: offHostCopyFunc(func(context.Context, backup.Snapshot) (backup.OffHost, error) {
			return backup.OffHost{Completed: true, CopiedAt: completedAt.Add(time.Minute)}, nil
		}),
		Restore: restoreVerifyFunc(func(_ context.Context, snapshot backup.Snapshot) (backup.RestoreEvidence, error) {
			return successfulRestore(snapshot, completedAt.Add(2*time.Minute)), nil
		}),
		Retention: retentionPlanFunc(func(context.Context, backup.RetentionInput) (backup.RetentionPlan, error) {
			return backup.RetentionPlan{}, nil
		}),
		Now: func() time.Time { return completedAt },
	}
}

func successfulRestore(snapshot backup.Snapshot, completedAt time.Time) backup.RestoreEvidence {
	return backup.RestoreEvidence{
		Isolated:    true,
		Outcome:     backup.RestoreOutcomeSucceeded,
		BackupSetID: snapshot.BackupSetID,
		CompletedAt: completedAt,
	}
}

func validBackupRequest(catalogDir, setID string) backup.BackupRequest {
	return backup.BackupRequest{
		CatalogDir:       catalogDir,
		BackupSetID:      setID,
		SystemIdentifier: "7439123456789012345",
	}
}

func assertCatalogEmpty(t *testing.T, catalogDir string) {
	t.Helper()
	entries, err := os.ReadDir(catalogDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("catalog contains partial output: %v", entries)
	}
}
