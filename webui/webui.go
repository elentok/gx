// Package webui holds the web UI assets embedded in the binary.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed placeholder
var placeholder embed.FS

// dist is the Vite build output (make web-build). Only dist/.gitkeep is
// committed, so the embed pattern always matches; "all:" includes dotfiles.
//
//go:embed all:dist
var dist embed.FS

// Assets is the built web UI when dist/index.html exists, else the placeholder page.
func Assets() fs.FS {
	return pick(dist, placeholder)
}

func pick(distFS, placeholderFS fs.FS) fs.FS {
	if _, err := fs.Stat(distFS, "dist/index.html"); err == nil {
		return sub(distFS, "dist")
	}
	return sub(placeholderFS, "placeholder")
}

func sub(fsys fs.FS, dir string) fs.FS {
	s, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err) // the directory name is fixed at compile time
	}
	return s
}
