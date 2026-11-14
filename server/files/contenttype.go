package files

import (
	"path/filepath"
	"strings"

	"github.com/elentok/gx/server/wire/ops"
)

// typeByExt is a fixed table, not the host's MIME database, so the answer is
// the same on every machine.
var typeByExt = map[string]string{
	".html": "text/html; charset=utf-8", ".htm": "text/html; charset=utf-8",
	".css": "text/css; charset=utf-8",
	".js":  "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8",
	".json": "application/json",
	".svg":  "image/svg+xml",
	".png":  "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
	".webp": "image/webp", ".avif": "image/avif", ".bmp": "image/bmp", ".ico": "image/x-icon",
	".pdf": "application/pdf",
}

// ContentType is the Content-Type of the file at path. Markdown and other text
// are text/plain; an unknown binary is application/octet-stream.
func ContentType(path string) string {
	if t, ok := typeByExt[strings.ToLower(filepath.Ext(path))]; ok {
		return t
	}
	if KindOf(path) == ops.HostedKindBinary {
		return "application/octet-stream"
	}
	return "text/plain; charset=utf-8"
}
