//go:build !linux

package database

import (
	"context"
	"os"
	"path/filepath"
)

// Non-Linux builds are development-only. Production Ubuntu uses the
// descriptor-bound /proc source in sqlite_restore_source_linux.go.
func prepareSQLiteRestoreSource(ctx context.Context, path string, pinned *os.File) (*sqliteRestoreSource, error) {
	size, digest, err := streamBoundedSQLiteRestoreSnapshot(ctx, path, pinned)
	if err != nil {
		return nil, err
	}
	return &sqliteRestoreSource{
		uri:    "file:" + filepath.ToSlash(path) + "?mode=ro&immutable=1",
		size:   size,
		digest: digest,
	}, nil
}
