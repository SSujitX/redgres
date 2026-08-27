// Package backupops wires the existing SQLite control-state backup
// primitives into the capture -> isolated restore verification lifecycle
// used by the redgres backup command (OPS-004 / NFR-010, first slice).
package backupops

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/SSujitX/redgres/internal/database"
)

// Result carries the captured snapshot and its isolated restore check.
type Result struct {
	Snapshot database.SQLiteSnapshot
	Check    database.SQLiteRestoreCheck
}

// Capture runs the existing online snapshot producer then verifies the
// snapshot with an isolated restore check. On verification failure it returns
// the error WITHOUT deleting the snapshot (retention/operator owns cleanup).
func Capture(ctx context.Context, source *sql.DB, stagingRoot string) (Result, error) {
	snapshot, err := database.CaptureSQLiteSnapshot(ctx, source, stagingRoot)
	if err != nil {
		return Result{}, fmt.Errorf("capture sqlite backup: %w", err)
	}
	check, err := database.VerifySQLiteSnapshotRestore(ctx, snapshot)
	if err != nil {
		return Result{}, fmt.Errorf("verify sqlite backup restore: %w", err)
	}
	return Result{Snapshot: snapshot, Check: check}, nil
}
