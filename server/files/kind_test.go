package files

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/wire/ops"
)

func TestKindOf_ExtensionTable(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]ops.HostedKind{
		"a.md": ops.HostedKindMarkdown, "a.MD": ops.HostedKindMarkdown,
		"a.mmd": ops.HostedKindMermaid, "a.html": ops.HostedKindHTML,
		"a.png": ops.HostedKindImage, "a.jpg": ops.HostedKindImage,
		"a.svg": ops.HostedKindSVG, "a.pdf": ops.HostedKindPDF,
		"a.go": ops.HostedKindCode, "a.ts": ops.HostedKindCode, "a.json": ops.HostedKindCode,
		"a.txt": ops.HostedKindText,
	}
	for name, want := range cases {
		if got := KindOf(filepath.Join(dir, name)); got != want {
			t.Errorf("KindOf(%s) = %q, want %q", name, got, want)
		}
	}
}

func TestKindOf_UnknownExtensionDecidedByBytes(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if got := KindOf(write("notes.zzz", []byte("héllo\nworld"))); got != ops.HostedKindText {
		t.Errorf("text bytes = %q, want text", got)
	}
	if got := KindOf(write("blob.zzz", []byte("ab\x00cd"))); got != ops.HostedKindBinary {
		t.Errorf("NUL bytes = %q, want binary", got)
	}
	if got := KindOf(write("bad.zzz", []byte{0xff, 0xfe, 'a'})); got != ops.HostedKindBinary {
		t.Errorf("invalid UTF-8 = %q, want binary", got)
	}
	if got := KindOf(write("empty.zzz", nil)); got != ops.HostedKindText {
		t.Errorf("empty = %q, want text", got)
	}
}
