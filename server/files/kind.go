package files

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/elentok/gx/server/wire/ops"
)

// extInfo is what a known extension decides. An empty contentType means
// text/plain (see ContentType).
type extInfo struct {
	kind        ops.HostedKind
	contentType string
}

// extTable is a fixed table, not the host's MIME database, so the answer is
// the same on every machine.
var extTable = buildExtTable()

func buildExtTable() map[string]extInfo {
	t := map[string]extInfo{}
	add := func(kind ops.HostedKind, contentType string, exts ...string) {
		for _, e := range exts {
			t[e] = extInfo{kind, contentType}
		}
	}
	const charset = "; charset=utf-8"
	add(ops.HostedKindMarkdown, "", ".md", ".markdown")
	add(ops.HostedKindMermaid, "", ".mmd", ".mermaid")
	add(ops.HostedKindHTML, "text/html"+charset, ".html", ".htm")
	add(ops.HostedKindImage, "image/png", ".png")
	add(ops.HostedKindImage, "image/jpeg", ".jpg", ".jpeg")
	add(ops.HostedKindImage, "image/gif", ".gif")
	add(ops.HostedKindImage, "image/webp", ".webp")
	add(ops.HostedKindImage, "image/avif", ".avif")
	add(ops.HostedKindImage, "image/bmp", ".bmp")
	add(ops.HostedKindImage, "image/x-icon", ".ico")
	add(ops.HostedKindSVG, "image/svg+xml", ".svg")
	add(ops.HostedKindPDF, "application/pdf", ".pdf")
	add(ops.HostedKindText, "", ".txt", ".log")
	add(ops.HostedKindCode, "text/css"+charset, ".css")
	add(ops.HostedKindCode, "text/javascript"+charset, ".js", ".mjs")
	add(ops.HostedKindCode, "application/json", ".json")
	add(ops.HostedKindCode, "",
		".go", ".ts", ".tsx", ".jsx", ".cjs", ".py", ".rb", ".rs", ".java", ".kt",
		".c", ".h", ".cc", ".cpp", ".hpp", ".cs", ".swift", ".php", ".lua", ".sh", ".bash", ".zsh",
		".fish", ".scss", ".yaml", ".yml", ".toml", ".xml", ".sql", ".vue", ".svelte")
	return t
}

// sniffLen is how many leading bytes decide text or binary for an unknown extension.
const sniffLen = 8192

// KindOf classifies the file at path. A known extension decides; an unknown one
// is text when the first bytes are valid UTF-8 with no NUL, else binary. The raw
// route reuses this one decision for its content type.
func KindOf(path string) ops.HostedKind {
	ext := strings.ToLower(filepath.Ext(path))
	if e, ok := extTable[ext]; ok {
		return e.kind
	}
	return sniffKind(path)
}

func sniffKind(path string) ops.HostedKind {
	f, err := os.Open(path)
	if err != nil {
		return ops.HostedKindBinary
	}
	defer f.Close()
	buf := make([]byte, sniffLen)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return ops.HostedKindBinary
	}
	buf = buf[:n]
	// A multi-byte rune cut by the sniff window is not invalid text.
	for i := 0; i < utf8.UTFMax && !utf8.Valid(buf) && len(buf) > 0 && n == sniffLen; i++ {
		buf = buf[:len(buf)-1]
	}
	if bytes.IndexByte(buf, 0) >= 0 || !utf8.Valid(buf) {
		return ops.HostedKindBinary
	}
	return ops.HostedKindText
}
