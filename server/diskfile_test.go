package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileFreeSpace(t *testing.T) {
	free := filepath.Join(t.TempDir(), "free")
	read := FileFreeSpace(free)

	if _, err := read("/a"); err == nil {
		t.Fatal("a missing file should be a read error")
	}
	if err := os.WriteFile(free, []byte("\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := read("/a"); err == nil {
		t.Fatal("an empty file should be a read error")
	}
	if err := os.WriteFile(free, []byte("15G\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := read("/a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := read("/elsewhere")
	if err != nil {
		t.Fatal(err)
	}
	if a.Free != 15_000_000_000 {
		t.Fatalf("free = %d, want 15G", a.Free)
	}
	if a != b {
		t.Fatalf("two paths read %+v and %+v, want one disk", a, b)
	}
}
