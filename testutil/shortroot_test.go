package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShortTempDir_LongTMPDIRStaysUnderSocketLimit(t *testing.T) {
	long := filepath.Join(t.TempDir(), strings.Repeat("d", 120))
	if err := os.MkdirAll(long, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", long)

	root, err := ShortTempDir("gxl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })

	sock := filepath.Join(root, strings.Repeat("x", HolderSocketBudget-len("/holder.sock")), "holder.sock")
	if len(sock) >= 100 {
		t.Fatalf("socket path is %d bytes, want under 100: %s", len(sock), sock)
	}
}

func TestShortTempDir_ShortTMPDIRIsKept(t *testing.T) {
	short := t.TempDir()
	if len(short) > 20 {
		short, _ = os.MkdirTemp("/tmp", "st")
		t.Cleanup(func() { os.RemoveAll(short) })
	}
	t.Setenv("TMPDIR", short)
	root, err := ShortTempDir("gxl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	if filepath.Dir(root) != short {
		t.Fatalf("root %s is not under short TMPDIR %s", root, short)
	}
}
