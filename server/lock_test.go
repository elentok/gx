package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockHeldBy(t *testing.T) {
	dir := t.TempDir()
	if _, held := LockHeldBy(dir); held {
		t.Fatal("no lock file: should not be held")
	}
	l, err := acquireLock(filepath.Join(dir, lockFileName))
	if err != nil {
		t.Fatal(err)
	}
	pid, held := LockHeldBy(dir)
	if !held || pid != os.Getpid() {
		t.Fatalf("held lock: got pid %d held %v", pid, held)
	}
	l.release()
	if _, held := LockHeldBy(dir); held {
		t.Fatal("released lock: should not be held")
	}
}
