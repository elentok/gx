// Package webui holds the web UI assets embedded in the binary.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed placeholder
var placeholder embed.FS

// Assets is the embedded web UI. It is the placeholder page until a build
// embeds a real dist.
func Assets() fs.FS {
	sub, err := fs.Sub(placeholder, "placeholder")
	if err != nil {
		panic(err) // the embedded directory is fixed at compile time
	}
	return sub
}
