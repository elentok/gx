package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreWatchSeesNewFilesAndSkipsGit(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := NewStoreWatch(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if err := os.WriteFile(filepath.Join(root, ".git", "objects", "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "epic")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	w.Rewatch()
	if err := os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-w.Events:
			if inGitDir(ev.Name) {
				t.Fatalf("got event inside .git: %v", ev)
			}
			if filepath.Base(ev.Name) == "01-a.md" {
				return
			}
		case <-deadline:
			t.Fatal("no event for the new ticket file")
		}
	}
}
