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

var kindByExt = map[string]ops.HostedKind{
	".md": ops.HostedKindMarkdown, ".markdown": ops.HostedKindMarkdown,
	".mmd": ops.HostedKindMermaid, ".mermaid": ops.HostedKindMermaid,
	".html": ops.HostedKindHTML, ".htm": ops.HostedKindHTML,
	".png": ops.HostedKindImage, ".jpg": ops.HostedKindImage, ".jpeg": ops.HostedKindImage,
	".gif": ops.HostedKindImage, ".webp": ops.HostedKindImage, ".avif": ops.HostedKindImage,
	".bmp": ops.HostedKindImage, ".ico": ops.HostedKindImage,
	".svg": ops.HostedKindSVG,
	".pdf": ops.HostedKindPDF,
	".txt": ops.HostedKindText, ".log": ops.HostedKindText,
}

var codeExts = []string{
	".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".py", ".rb", ".rs", ".java", ".kt",
	".c", ".h", ".cc", ".cpp", ".hpp", ".cs", ".swift", ".php", ".lua", ".sh", ".bash", ".zsh",
	".fish", ".css", ".scss", ".json", ".yaml", ".yml", ".toml", ".xml", ".sql", ".vue", ".svelte",
}

// sniffLen is how many leading bytes decide text or binary for an unknown extension.
const sniffLen = 8192

// KindOf classifies the file at path. A known extension decides; an unknown one
// is text when the first bytes are valid UTF-8 with no NUL, else binary. The raw
// route reuses this one decision for its content type.
func KindOf(path string) ops.HostedKind {
	ext := strings.ToLower(filepath.Ext(path))
	if k, ok := kindByExt[ext]; ok {
		return k
	}
	for _, c := range codeExts {
		if c == ext {
			return ops.HostedKindCode
		}
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
