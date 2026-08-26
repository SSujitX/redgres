package database

import "io/fs"

// Windows is a development-only platform for this producer. The production
// Ubuntu path enforces owner/mode/ancestry in sqlite_backup_security_unix.go.
func verifySnapshotAncestorSecurity(fs.FileInfo, bool) error { return nil }
