package backupops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// snapshotOperationName matches exactly the producer namespace used by
// database.CaptureSQLiteSnapshot: "redgres-sqlite-" followed by 48 lowercase
// hex characters (24 random bytes hex-encoded).
var snapshotOperationName = regexp.MustCompile(`^redgres-sqlite-[0-9a-f]{48}$`)

// ApplyRetention prunes producer-owned snapshot operation directories under
// stagingRoot, keeping the keep most recent and never deleting the most
// recent directory. It returns the relative names it removed (for operator
// logging) and an error. Never follow symlinks; never touch entries that do
// not exactly match the producer namespace; never delete the newest match.
func ApplyRetention(ctx context.Context, stagingRoot string, keep int) (removed []string, err error) {
	if keep < 1 {
		return nil, errors.New("apply retention: keep must be at least 1")
	}
	if stagingRoot == "" || !filepath.IsAbs(stagingRoot) {
		return nil, errors.New("apply retention: staging root must be an absolute path")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(stagingRoot)
	if err != nil {
		return nil, fmt.Errorf("apply retention: read staging root: %w", err)
	}

	type candidate struct {
		name string
		path string
		mod  time.Time
	}
	var snapshots []candidate
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := entry.Name()
		if !snapshotOperationName.MatchString(name) {
			// Never touch entries outside the producer namespace.
			continue
		}
		info, err := os.Lstat(filepath.Join(stagingRoot, name))
		if err != nil {
			return nil, fmt.Errorf("apply retention: inspect %q: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			// Not a real producer directory: never follow symlinks, never touch.
			continue
		}
		snapshots = append(snapshots, candidate{
			name: name,
			path: filepath.Join(stagingRoot, name),
			mod:  info.ModTime(),
		})
	}
	if len(snapshots) <= keep {
		return nil, nil
	}

	// Newest first. Removing from index keep onward can never touch the
	// newest directory because keep >= 1.
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].mod.After(snapshots[j].mod) })
	for _, snapshot := range snapshots[keep:] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Re-verify identity immediately before removal so a directory swapped
		// for a symlink is never followed.
		info, err := os.Lstat(snapshot.path)
		if err != nil {
			return nil, fmt.Errorf("apply retention: inspect %q: %w", snapshot.name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, fmt.Errorf("apply retention: %q is no longer a real directory", snapshot.name)
		}
		if err := os.RemoveAll(snapshot.path); err != nil {
			return nil, fmt.Errorf("apply retention: remove %q: %w", snapshot.name, err)
		}
		removed = append(removed, snapshot.name)
	}
	return removed, nil
}
