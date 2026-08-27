package backupops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"time"
)

// snapshotOperationName matches exactly the producer namespace used by
// database.CaptureSQLiteSnapshot: "redgres-sqlite-" followed by 48 lowercase
// hex characters (24 random bytes hex-encoded).
var snapshotOperationName = regexp.MustCompile(`^redgres-sqlite-[0-9a-f]{48}$`)

// ApplyRetention prunes producer-owned snapshot operation directories under
// stagingRoot, keeping the keep most recent and never deleting the most
// recent directory. protectName is a producer operation directory name that is
// additionally never removed (the caller passes the just-captured snapshot so
// clock skew or a future-mtime sibling cannot prune it in the same run); an
// empty protectName disables that protection. It returns the relative names it
// removed (for operator logging) and an error.
//
// The staging root is validated, pinned with os.OpenRoot, and re-verified by
// identity before every destructive step, so a root path swapped for a symlink
// after pinning can never redirect a removal outside the root. Removal is
// performed only through the pinned handle (never a lexically re-resolved
// path), and only real directories exactly matching the producer namespace are
// ever touched.
func ApplyRetention(ctx context.Context, stagingRoot string, keep int, protectName string) (removed []string, err error) {
	if keep < 1 {
		return nil, errors.New("apply retention: keep must be at least 1")
	}
	if stagingRoot == "" || !filepath.IsAbs(stagingRoot) {
		return nil, errors.New("apply retention: staging root must be an absolute path")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	root, identity, err := openRetentionRoot(stagingRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	dir, err := root.Open(".")
	if err != nil {
		return nil, fmt.Errorf("apply retention: open staging root: %w", err)
	}
	entries, readErr := dir.ReadDir(-1)
	closeErr := dir.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("apply retention: read staging root: %w", errors.Join(readErr, closeErr))
	}

	type candidate struct {
		name string
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
		info, err := root.Lstat(name)
		if err != nil {
			return nil, fmt.Errorf("apply retention: inspect %q: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			// Not a real producer directory: never follow symlinks, never touch.
			continue
		}
		snapshots = append(snapshots, candidate{name: name, mod: info.ModTime()})
	}
	if len(snapshots) <= keep {
		return nil, nil
	}

	// Newest first; deterministic tiebreak so equal mtimes sort stably.
	sort.Slice(snapshots, func(i, j int) bool {
		if !snapshots[i].mod.Equal(snapshots[j].mod) {
			return snapshots[i].mod.After(snapshots[j].mod)
		}
		return snapshots[i].name < snapshots[j].name
	})

	for _, snapshot := range snapshots[keep:] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if snapshot.name == protectName {
			// Never delete the protected (just-captured) snapshot.
			continue
		}
		// Re-verify the pinned root identity and the candidate's real-directory
		// identity immediately before removal, all through the pinned handle.
		if err := verifyRetentionRootIdentity(stagingRoot, identity, root); err != nil {
			return nil, err
		}
		info, err := root.Lstat(snapshot.name)
		if err != nil {
			return nil, fmt.Errorf("apply retention: inspect %q: %w", snapshot.name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, fmt.Errorf("apply retention: %q is no longer a real directory", snapshot.name)
		}
		if err := root.RemoveAll(snapshot.name); err != nil {
			return nil, fmt.Errorf("apply retention: remove %q: %w", snapshot.name, err)
		}
		removed = append(removed, snapshot.name)
	}
	return removed, nil
}

// openRetentionRoot validates and pins the staging root, returning the pinned
// handle and its Lstat identity captured before pinning.
func openRetentionRoot(path string) (*os.Root, fs.FileInfo, error) {
	if err := verifyRetentionRootPath(path); err != nil {
		return nil, nil, err
	}
	identity, err := os.Lstat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("apply retention: staging root: %w", err)
	}
	if identity.Mode()&os.ModeSymlink != 0 || !identity.IsDir() {
		return nil, nil, errors.New("apply retention: staging root must be a real directory")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, nil, fmt.Errorf("apply retention: open staging root: %w", err)
	}
	return root, identity, nil
}

// verifyRetentionRootPath requires the staging root and every ancestor to be a
// real directory (no symlinks), and on non-Windows requires the root itself to
// grant no group/other permissions (mirroring the producer's root policy).
func verifyRetentionRootPath(path string) error {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("apply retention: staging root ancestry: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("apply retention: staging root ancestry must contain only real directories")
		}
		if current == filepath.Clean(path) && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return errors.New("apply retention: staging root must not grant group or other permissions")
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
	}
}

// verifyRetentionRootIdentity re-checks that the lexical root path still
// resolves to the same directory that was pinned, using both the path Lstat
// and the pinned handle, before a destructive step.
func verifyRetentionRootIdentity(path string, identity fs.FileInfo, root *os.Root) error {
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("apply retention: staging root changed: %w", err)
	}
	opened, err := root.Stat(".")
	if err != nil {
		return fmt.Errorf("apply retention: staging root changed: %w", err)
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !opened.IsDir() ||
		!os.SameFile(identity, current) || !os.SameFile(identity, opened) {
		return errors.New("apply retention: staging root changed during retention")
	}
	return nil
}
