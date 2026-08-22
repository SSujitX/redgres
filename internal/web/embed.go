package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func Assets() (fs.FS, error) {
	return fs.Sub(dist, "dist/app")
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
