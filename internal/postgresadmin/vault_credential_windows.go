//go:build windows

package postgresadmin

import "io/fs"

func rootOwnedFile(fs.FileInfo) bool { return false }
