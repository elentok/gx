package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// realWatchDeadline bounds the real-filesystem smoke tests well under the
// 20 s slow limit. The behaviour itself is pinned by fake-stream tests.
const realWatchDeadline = 8 * time.Second

func TestStoreWatchSeesNewFilesAndSkipsGit(t *testing.T) {
	t.Parallel()
	testStoreWatchSeesNewFilesAndSkipsGit(t, NewStoreWatch)
}

func TestFSNotifyStoreWatchSeesNewFilesAndSkipsGit(t *testing.T) {
	t.Parallel()
	testStoreWatchSeesNewFilesAndSkipsGit(t, newFSNotifyStoreWatch)
}

func testStoreWatchSeesNewFilesAndSkipsGit(t *testing.T, newWatch func(string) (*StoreWatch, error)) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := newWatch(root)
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
	deadline := time.After(realWatchDeadline)
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
			t.Fatal("environment problem, not a product bug: the OS delivered no file event (see gx:follow-ups/499)")
		}
	}
}

func TestFSNotifyStoreWatchCloseWithoutReadingEvents(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	w, err := newFSNotifyStoreWatch(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "01-a.md"), []byte("ticket"), 0o644); err != nil {
		w.Close()
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() {
		w.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher and error reader did not stop")
	}
}
