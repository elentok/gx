package server

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// holdLock takes the flock on its own fd and writes pid into the file.
func holdLock(t *testing.T, path, pid string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte(pid), 0); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestAcquireLock_SamePidRetriesUntilFree(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), lockFileName)
	held := holdLock(t, path, strconv.Itoa(os.Getpid()))
	time.AfterFunc(200*time.Millisecond, func() { held.Close() })

	l, err := acquireLock(path)
	if err != nil {
		t.Fatalf("same-pid lock freed mid-retry: %v", err)
	}
	l.release()
}

func TestAcquireLock_OtherPidFailsAtOnce(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), lockFileName)
	held := holdLock(t, path, "999999")
	defer held.Close()

	start := time.Now()
	_, err := acquireLock(path)
	if err == nil || !strings.Contains(err.Error(), "already running (pid 999999)") {
		t.Fatalf("got %v, want already running (pid 999999)", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("other pid should fail at once, took %v", d)
	}
}

func TestLockHeldBy(t *testing.T) {
	t.Parallel()
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
	// A process forked by a parallel test can still hold the inherited fd for a
	// moment after release, as the same-pid retry in acquireLock expects.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, held := LockHeldBy(dir); !held {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("released lock: should not be held")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
