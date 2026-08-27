package backupops

import (
	"os"
	"testing"
)

// secureStagingRoot returns a staging root that satisfies CaptureSQLiteSnapshot's
// Unix ownership and mode requirements (mode 0700, no group/other permissions).
// t.TempDir() is not guaranteed to be 0700 on every CI filesystem, so chmod it
// explicitly; on Windows chmod is a no-op and the Unix checks are skipped anyway.
func secureStagingRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatalf("chmod staging root: %v", err)
	}
	return root
}
