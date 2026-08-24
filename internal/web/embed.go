package web

import (
	"embed"
	"io"
	"io/fs"
	"time"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the embedded frontend assets. A build that ran without the
// frontend embeds only the tracked dist/.gitkeep placeholder, so dist/app does
// not exist; Assets then reports a valid empty tree instead of an unusable one.
// Callers still find no index.html, so the HTTP layer reports the frontend as
// unavailable rather than serving a partial application.
func Assets() (fs.FS, error) {
	assets, err := fs.Sub(dist, "dist/app")
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(assets, "."); err != nil {
		return emptyFS{}, nil
	}
	return assets, nil
}

func Exists(name string) bool {
	assets, err := Assets()
	if err != nil {
		return false
	}
	f, err := assets.Open(name)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// emptyFS is a readable filesystem containing only its root directory.
type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &emptyDir{}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (fsys emptyFS) Stat(name string) (fs.FileInfo, error) {
	if name == "." {
		return emptyDirInfo{}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

func (emptyFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "." {
		return nil, nil
	}
	return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
}

type emptyDir struct{}

func (*emptyDir) Stat() (fs.FileInfo, error) { return emptyDirInfo{}, nil }
func (*emptyDir) Close() error               { return nil }

func (*emptyDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: ".", Err: fs.ErrInvalid}
}

func (*emptyDir) ReadDir(int) ([]fs.DirEntry, error) { return nil, io.EOF }

type emptyDirInfo struct{}

func (emptyDirInfo) Name() string       { return "." }
func (emptyDirInfo) Size() int64        { return 0 }
func (emptyDirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o500 }
func (emptyDirInfo) ModTime() time.Time { return time.Time{} }
func (emptyDirInfo) IsDir() bool        { return true }
func (emptyDirInfo) Sys() any           { return nil }
