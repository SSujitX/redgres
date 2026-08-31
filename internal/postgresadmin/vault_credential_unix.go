//go:build !windows

package postgresadmin

import (
	"io/fs"
	"syscall"
)

func rootOwnedFile(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && stat.Gid == 0
}
