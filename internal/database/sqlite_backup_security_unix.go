//go:build !windows

package database

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func verifySnapshotAncestorSecurity(info fs.FileInfo, stagingRoot bool) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("staging root ownership is unavailable")
	}
	effectiveUID := os.Geteuid()
	if stagingRoot {
		if int(stat.Uid) != effectiveUID {
			return fmt.Errorf("staging root must be owned by the effective user")
		}
		if info.Mode().Perm() != 0o700 {
			return fmt.Errorf("staging root permissions must be 0700")
		}
		return nil
	}
	if int(stat.Uid) != 0 && int(stat.Uid) != effectiveUID {
		return fmt.Errorf("staging root ancestry must be owned by root or the effective user")
	}
	if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
		return fmt.Errorf("staging root ancestry must not be writable by group or other users")
	}
	return nil
}
