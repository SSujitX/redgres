package backup

import (
	"path/filepath"
	"strings"
)

func jailLocal(path string) bool {
	if path == "" || strings.ContainsRune(path, 0) || strings.ContainsAny(path, "?#%") {
		return false
	}
	if filepath.IsAbs(path) || filepath.IsAbs(filepath.FromSlash(path)) {
		return false
	}
	for _, part := range strings.FieldsFunc(path, isPathSeparator) {
		if part == ".." {
			return false
		}
	}
	return filepath.IsLocal(filepath.FromSlash(path))
}

func isPathSeparator(r rune) bool {
	return r == '/' || r == '\\'
}
