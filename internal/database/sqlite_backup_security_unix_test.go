//go:build !windows

package database

import (
	"io/fs"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestVerifySnapshotAncestorSecurityRejectsUntrustedOwner(t *testing.T) {
	otherUID := uint32(os.Geteuid() + 1)
	ancestor := syntheticSnapshotDir{mode: fs.ModeDir | 0o755, stat: &syscall.Stat_t{Uid: otherUID}}
	if err := verifySnapshotAncestorSecurity(ancestor, false); err == nil {
		t.Fatal("expected unrelated ancestor owner rejection")
	}
}

func TestVerifySnapshotAncestorSecurityAcceptsTrustedOwners(t *testing.T) {
	for name, uid := range map[string]uint32{
		"root":           0,
		"effective-user": uint32(os.Geteuid()),
	} {
		t.Run(name, func(t *testing.T) {
			ancestor := syntheticSnapshotDir{mode: fs.ModeDir | 0o755, stat: &syscall.Stat_t{Uid: uid}}
			if err := verifySnapshotAncestorSecurity(ancestor, false); err != nil {
				t.Fatalf("trusted ancestor rejected: %v", err)
			}
		})
	}
}

type syntheticSnapshotDir struct {
	mode fs.FileMode
	stat *syscall.Stat_t
}

func (syntheticSnapshotDir) Name() string        { return "ancestor" }
func (syntheticSnapshotDir) Size() int64         { return 0 }
func (i syntheticSnapshotDir) Mode() fs.FileMode { return i.mode }
func (syntheticSnapshotDir) ModTime() time.Time  { return time.Time{} }
func (syntheticSnapshotDir) IsDir() bool         { return true }
func (i syntheticSnapshotDir) Sys() any          { return i.stat }
