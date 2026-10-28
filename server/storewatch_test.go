package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreWatchSeesNewFilesAndSkipsGit(t *testing.T) {
	t.Parallel()
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

	// Under heavy disk load FSEvents can delay a write by seconds or drop it,
	// so keep writing until one gets through.
	write := time.NewTicker(500 * time.Millisecond)
	defer write.Stop()
	deadline := time.After(20 * time.Second)
	writeTicket := func(i int) {
		if err := os.WriteFile(filepath.Join(dir, "01-a.md"), []byte{byte(i)}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeTicket(0)
	for i := 1; ; i++ {
		select {
		case ev := <-w.Events:
			if inGitDir(ev.Name) {
				t.Fatalf("got event inside .git: %v", ev)
			}
			if filepath.Base(ev.Name) == "01-a.md" {
				return
			}
		case <-write.C:
			writeTicket(i)
		case <-deadline:
			t.Fatal("no event for the new ticket file")
		}
	}
}
