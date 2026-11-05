package lifecycle_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/server/lifecycle"
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
	path := filepath.Join(t.TempDir(), lifecycle.LockFileName)
	held := holdLock(t, path, strconv.Itoa(os.Getpid()))
	time.AfterFunc(200*time.Millisecond, func() { held.Close() })

	l, err := lifecycle.AcquireLock(path)
	if err != nil {
		t.Fatalf("same-pid lock freed mid-retry: %v", err)
	}
	l.Release()
}

func TestAcquireLock_OtherPidFailsAtOnce(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), lifecycle.LockFileName)
	held := holdLock(t, path, "999999")
	defer held.Close()

	start := time.Now()
	_, err := lifecycle.AcquireLock(path)
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
	if _, held := lifecycle.LockHeldBy(dir); held {
		t.Fatal("no lock file: should not be held")
	}
	l, err := lifecycle.AcquireLock(filepath.Join(dir, lifecycle.LockFileName))
	if err != nil {
		t.Fatal(err)
	}
	pid, held := lifecycle.LockHeldBy(dir)
	if !held || pid != os.Getpid() {
		t.Fatalf("held lock: got pid %d held %v", pid, held)
	}
	l.Release()
	// A process forked by a parallel test can still hold the inherited fd for a
	// moment after release, as the same-pid retry in AcquireLock expects.
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, held := lifecycle.LockHeldBy(dir); !held {
			return
		}
		select {
		case <-deadline:
			t.Fatal("released lock: should not be held")
		case <-tick.C:
		}
	}
}
