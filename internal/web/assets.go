package web

import (
	"io/fs"
	"os"
)

// Open returns the frontend asset filesystem and a release function. An empty
// devAssetDir selects the embedded assets. A non-empty value is a
// development-only directory that internal/config has already validated and
// rejected for production, so this package reads no environment and decides no
// policy.
//
// The directory is opened as an os.Root so a symlink or junction inside it
// cannot resolve to a target outside it; os.DirFS would follow such a link.
// The returned function closes the directory handle that os.Root holds open.
func Open(devAssetDir string) (fs.FS, func(), error) {
	noop := func() {}
	if devAssetDir == "" {
		assets, err := Assets()
		if err != nil {
			return nil, noop, err
		}
		return assets, noop, nil
	}
	root, err := os.OpenRoot(devAssetDir)
	if err != nil {
		return nil, noop, err
	}
	return root.FS(), func() { _ = root.Close() }, nil
}
