package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvaluateDropGateAllowsFreshMatchedSet(t *testing.T) {
	manifest := mustParse(t, "valid-fresh.json")
	manifest.Restore.CompletedAt = manifest.CompletedAt.Add(time.Minute)
	result := EvaluateDropGate(DropGateInput{
		Database:         "appdb",
		SystemIdentifier: "7439123456789012345",
		Now:              evalNow,
		Manifest:         manifest,
	})
	if !result.Allowed {
		t.Fatalf("Allowed = false, reason %q", result.Reason)
	}
	if result.Reason != "" {
		t.Fatalf("Reason = %q, want empty", result.Reason)
	}
}

func TestEvaluateDropGateRejectsImpossibleEvidenceChronology(t *testing.T) {
	base := mustParse(t, "valid-fresh.json")
	base.Restore.CompletedAt = base.CompletedAt.Add(time.Minute)
	cases := []struct {
		name   string
		mutate func(*Manifest)
		reason string
	}{
		{
			name: "future completed_at",
			mutate: func(manifest *Manifest) {
				manifest.CompletedAt = evalNow.Add(time.Nanosecond)
				manifest.OffHost.CopiedAt = manifest.CompletedAt
				manifest.Restore.CompletedAt = manifest.CompletedAt
			},
			reason: reasonInvalidManifest,
		},
		{
			name: "future copied_at",
			mutate: func(manifest *Manifest) {
				manifest.OffHost.CopiedAt = evalNow.Add(time.Nanosecond)
			},
			reason: reasonOffHost,
		},
		{
			name: "restore before completed_at",
			mutate: func(manifest *Manifest) {
				manifest.Restore.CompletedAt = manifest.CompletedAt.Add(-time.Nanosecond)
			},
			reason: reasonRestore,
		},
		{
			name: "future restore completed_at",
			mutate: func(manifest *Manifest) {
				manifest.Restore.CompletedAt = evalNow.Add(time.Nanosecond)
			},
			reason: reasonRestore,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := base
			tc.mutate(&manifest)
			result := EvaluateDropGate(DropGateInput{
				Database:         "appdb",
				SystemIdentifier: "7439123456789012345",
				Now:              evalNow,
				Manifest:         manifest,
			})
			if result.Allowed {
				t.Fatal("Allowed = true, want fail closed")
			}
			if result.Reason != tc.reason {
				t.Fatalf("Reason = %q, want %q", result.Reason, tc.reason)
			}
		})
	}
}

func TestEvaluateDropGateAllowsEvidenceChronologyBoundaries(t *testing.T) {
	base := mustParse(t, "valid-fresh.json")
	cases := []struct {
		name     string
		copiedAt time.Time
		restored time.Time
	}{
		{name: "evidence at completed_at", copiedAt: base.CompletedAt, restored: base.CompletedAt},
		{name: "evidence at Now", copiedAt: evalNow, restored: evalNow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := base
			manifest.OffHost.CopiedAt = tc.copiedAt
			manifest.Restore.CompletedAt = tc.restored
			result := EvaluateDropGate(DropGateInput{
				Database:         "appdb",
				SystemIdentifier: "7439123456789012345",
				Now:              evalNow,
				Manifest:         manifest,
			})
			if !result.Allowed {
				t.Fatalf("Allowed = false, reason %q", result.Reason)
			}
		})
	}
}

func TestEvaluateDropGateFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		db      string
		sysID   string
	}{
		{name: "stale completed_at", fixture: "stale-completed.json", db: "appdb", sysID: "7439123456789012345"},
		{name: "wrong system_identifier", fixture: "wrong-system-id.json", db: "appdb", sysID: "7439123456789012345"},
		{name: "wrong database name", fixture: "wrong-database.json", db: "appdb", sysID: "7439123456789012345"},
		{name: "missing off-host", fixture: "missing-off-host.json", db: "appdb", sysID: "7439123456789012345"},
		{name: "copied_at before completed_at", fixture: "copied-at-before.json", db: "appdb", sysID: "7439123456789012345"},
		{name: "stale restore", fixture: "stale-restore.json", db: "appdb", sysID: "7439123456789012345"},
		{name: "restore backup_set_id mismatch", fixture: "restore-set-mismatch.json", db: "appdb", sysID: "7439123456789012345"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := mustParse(t, tc.fixture)
			result := EvaluateDropGate(DropGateInput{
				Database:         tc.db,
				SystemIdentifier: tc.sysID,
				Now:              evalNow,
				Manifest:         manifest,
			})
			if result.Allowed {
				t.Fatal("Allowed = true, want fail closed")
			}
			if result.Reason == "" {
				t.Fatal("Reason is empty")
			}
			if strings.Contains(result.Reason, "..") || strings.Contains(result.Reason, "canary-secret") {
				t.Fatalf("Reason leaked unsafe content: %q", result.Reason)
			}
		})
	}
}

func TestEvaluateDropGateAgeBoundaries(t *testing.T) {
	base := mustParse(t, "valid-fresh.json")
	base.Restore.CompletedAt = base.CompletedAt.Add(time.Minute)
	input := DropGateInput{
		Database:         "appdb",
		SystemIdentifier: "7439123456789012345",
		Manifest:         base,
	}

	t.Run("backup age equal 24h allowed", func(t *testing.T) {
		input.Now = base.CompletedAt.Add(BackupMaxAge)
		result := EvaluateDropGate(input)
		if !result.Allowed {
			t.Fatalf("Allowed = false at 24h, reason %q", result.Reason)
		}
	})
	t.Run("backup age over 24h denied", func(t *testing.T) {
		input.Now = base.CompletedAt.Add(BackupMaxAge + time.Nanosecond)
		result := EvaluateDropGate(input)
		if result.Allowed {
			t.Fatal("Allowed = true just over 24h")
		}
	})

}

func TestEvaluateDropGateFailsClosedOnZeroTimes(t *testing.T) {
	base := mustParse(t, "valid-fresh.json")

	t.Run("zero Now", func(t *testing.T) {
		result := EvaluateDropGate(DropGateInput{
			Database:         "appdb",
			SystemIdentifier: "7439123456789012345",
			Manifest:         base,
		})
		if result.Allowed {
			t.Fatal("Allowed = true with zero Now")
		}
		if result.Reason != reasonInvalidManifest {
			t.Fatalf("Reason = %q, want %q", result.Reason, reasonInvalidManifest)
		}
	})
	t.Run("zero restore completed_at", func(t *testing.T) {
		manifest := base
		manifest.Restore.CompletedAt = time.Time{}
		result := EvaluateDropGate(DropGateInput{
			Database:         "appdb",
			SystemIdentifier: "7439123456789012345",
			Now:              evalNow,
			Manifest:         manifest,
		})
		if result.Allowed {
			t.Fatal("Allowed = true with zero restore completed_at")
		}
		if result.Reason != reasonRestore {
			t.Fatalf("Reason = %q, want %q", result.Reason, reasonRestore)
		}
	})
}

func TestEvaluateDropGateRejectsTraversalWithoutLeakingPath(t *testing.T) {
	manifest := mustParse(t, "valid-fresh.json")
	manifest.Artifacts[0].Path = "../escape.dump"
	result := EvaluateDropGate(DropGateInput{
		Database:         "appdb",
		SystemIdentifier: "7439123456789012345",
		Now:              evalNow,
		Manifest:         manifest,
	})
	if result.Allowed {
		t.Fatal("Allowed = true for traversal artifact path")
	}
	if strings.Contains(result.Reason, "..") || strings.Contains(result.Reason, "escape.dump") {
		t.Fatalf("Reason leaked path: %q", result.Reason)
	}
}

func TestEvaluateDropGateAllowsWhenArtifactBytesMissing(t *testing.T) {
	dir := writeCatalog(t, "valid-fresh.json")
	if _, err := os.Stat(filepath.Join(dir, "artifacts", "appdb.dump")); !os.IsNotExist(err) {
		t.Fatalf("artifact file should be absent: %v", err)
	}
	manifest, err := ParseManifest(dir, "manifest.json")
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	manifest.Restore.CompletedAt = manifest.CompletedAt.Add(time.Minute)
	result := EvaluateDropGate(DropGateInput{
		Database:         "appdb",
		SystemIdentifier: "7439123456789012345",
		Now:              evalNow,
		Manifest:         manifest,
	})
	if !result.Allowed {
		t.Fatalf("Allowed = false when artifact bytes are missing, reason %q", result.Reason)
	}
}

var evalNow = time.Date(2026, 8, 26, 5, 0, 0, 0, time.UTC)

func mustParse(t *testing.T, fixture string) Manifest {
	t.Helper()
	dir := writeCatalog(t, fixture)
	manifest, err := ParseManifest(dir, "manifest.json")
	if err != nil {
		t.Fatalf("ParseManifest(%s): %v", fixture, err)
	}
	return manifest
}
