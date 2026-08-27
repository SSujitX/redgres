package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/SSujitX/redgres/internal/backupops"
	"github.com/SSujitX/redgres/internal/database"
)

// runBackup captures the SQLite control-state snapshot, verifies it with an
// isolated restore check, and prunes old snapshots by retention (OPS-004 /
// NFR-010, first slice: no manifest publication, PostgreSQL/Redis dump, or
// off-host copy).
func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	sqlitePath := fs.String("sqlite-path", envDefault("REDGRES_SQLITE_PATH", "./redgres.db"), "SQLite database path")
	stagingRoot := fs.String("staging-root", "", "absolute staging root for SQLite snapshots (required)")
	keep := fs.Int("keep", 7, "number of most recent snapshots to retain")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *stagingRoot == "" {
		return errors.New("backup: -staging-root is required")
	}
	if !filepath.IsAbs(*stagingRoot) {
		return errors.New("backup: -staging-root must be an absolute path")
	}
	if *keep < 1 {
		return errors.New("backup: -keep must be at least 1")
	}
	if info, err := os.Lstat(*stagingRoot); err != nil {
		return fmt.Errorf("backup: staging root: %w", err)
	} else if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("backup: staging root must be a real directory")
	}
	if !filepath.IsAbs(*sqlitePath) {
		return errors.New("backup: -sqlite-path must be an absolute path")
	}
	if info, err := os.Lstat(*sqlitePath); err != nil {
		return errors.New("backup: sqlite database does not exist")
	} else if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("backup: sqlite database must be a regular file")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(*sqlitePath)
	if err != nil {
		return fmt.Errorf("backup: open sqlite database: %w", err)
	}
	defer db.Close()

	result, err := backupops.Capture(ctx, db, *stagingRoot)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	protectName := filepath.Base(filepath.Dir(result.Snapshot.Path))
	removed, err := backupops.ApplyRetention(ctx, *stagingRoot, *keep, protectName)
	if err != nil {
		return fmt.Errorf("backup: apply retention: %w", err)
	}

	printBackupSummary(result, removed)
	return nil
}

func printBackupSummary(result backupops.Result, removed []string) {
	out := os.Stdout
	fmt.Fprintln(out, "backup ok")
	fmt.Fprintf(out, "snapshot sha256: %s\n", result.Snapshot.SHA256)
	fmt.Fprintf(out, "snapshot size_bytes: %d\n", result.Snapshot.SizeBytes)
	fmt.Fprintf(out, "schema_version: %d\n", result.Check.SchemaVersion)
	fmt.Fprintf(out, "owners: %d\n", result.Check.OwnerCount)
	fmt.Fprintf(out, "sessions: %d\n", result.Check.SessionCount)
	fmt.Fprintf(out, "login_attempts: %d\n", result.Check.LoginAttemptCount)
	fmt.Fprintf(out, "audit_events: %d\n", result.Check.AuditEventCount)
	fmt.Fprintf(out, "operations: %d\n", result.Check.OperationCount)
	fmt.Fprintf(out, "operation_locks: %d\n", result.Check.OperationLockCount)
	if len(removed) > 0 {
		fmt.Fprintf(out, "retention removed: %s\n", strings.Join(removed, " "))
	}
}
