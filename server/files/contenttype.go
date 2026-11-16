package files

import (
	"path/filepath"
	"strings"

	"github.com/elentok/gx/server/wire/ops"
)

// ContentType is the Content-Type of the file at path. Markdown and other text
// are text/plain; an unknown binary is application/octet-stream.
func ContentType(path string) string {
	if e := extTable[strings.ToLower(filepath.Ext(path))]; e.contentType != "" {
		return e.contentType
	}
	if KindOf(path) == ops.HostedKindBinary {
		return "application/octet-stream"
	}
	return "text/plain; charset=utf-8"
}
