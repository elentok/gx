package transcript

import (
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// FileCall is a file tool's name and file_path as the call wrote it. It is zero
// for every other call.
type FileCall struct {
	Tool string
	Path string
}

// LabelIn returns the label a session working in dir shows for the call, and
// the part of that label that names the file, empty for other calls. The label
// is built here, at read time: parsed calls are cached per file and the parsers
// do not know the session.
func (t InFlightTool) LabelIn(dir string) (label, path string) {
	if t.File.Path == "" {
		return t.Label, ""
	}
	path = pathWithin(dir, t.File.Path)
	prefix := t.File.Tool + " "
	path = shortenPath(path, inFlightLabelCap-utf8.RuneCountInString(prefix))
	return prefix + path, path
}

// pathWithin is path relative to dir when it is inside it, else path unchanged.
func pathWithin(dir, path string) string {
	if !filepath.IsAbs(dir) || !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return path
	}
	return rel
}

// shortenPath fits path into limit runes by dropping whole folders from its
// start, and then the start of the file name.
func shortenPath(path string, limit int) string {
	const cut = "…"
	if utf8.RuneCountInString(path) <= limit {
		return path
	}
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		if s := cut + "/" + strings.Join(parts[i:], "/"); utf8.RuneCountInString(s) <= limit {
			return s
		}
	}
	name := []rune(parts[len(parts)-1])
	return cut + string(name[max(0, len(name)-limit+1):])
}
