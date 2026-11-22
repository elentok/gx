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

func padPid(pid string) string {
	return strings.Repeat("0", lifecycle.LockPidWidth-len(pid)) + pid
}

// holdLock takes the flock on its own fd and writes the padded pid into the file.
func holdLock(t *testing.T, path, pid string) *os.File {
	t.Helper()
	return holdLockDelayed(t, path, pid, 0)
}

// holdLockDelayed takes the flock at once and writes the padded pid after delay.
func holdLockDelayed(t *testing.T, path, pid string, delay time.Duration) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	write := func() { _, _ = f.WriteAt([]byte(padPid(pid)), 0) }
	if delay == 0 {
		write()
	} else {
		time.AfterFunc(delay, write)
	}
	return f
}

func TestAcquireLock_NewFileGapNeverReportsEmptyPid(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), lifecycle.LockFileName)
	held := holdLockDelayed(t, path, "999999", 100*time.Millisecond)
	defer held.Close()

	_, err := lifecycle.AcquireLock(path)
	if err == nil || !strings.Contains(err.Error(), "already running (pid 999999)") {
		t.Fatalf("got %v, want already running (pid 999999)", err)
	}
}

func TestAcquireLock_NeverTruncatesTheFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), lifecycle.LockFileName)
	if err := os.WriteFile(path, []byte("1234567890"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan int)
	go func() {
		bad := 0
		for {
			select {
			case <-stop:
				done <- bad
				return
			default:
			}
			if raw, err := os.ReadFile(path); err == nil && len(raw) < lifecycle.LockPidWidth {
				bad++
			}
		}
	}()
	for range 2000 {
		l, err := lifecycle.AcquireLock(path)
		if err != nil {
			t.Fatal(err)
		}
		l.Release()
	}
	close(stop)
	if bad := <-done; bad != 0 {
		t.Fatalf("reader saw %d empty or short reads", bad)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != padPid(strconv.Itoa(os.Getpid())) {
		t.Fatalf("final content %q", raw)
	}
}

func TestAcquireLock_WritesFixedWidthPid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, lifecycle.LockFileName)
	l, err := lifecycle.AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	raw, _ := os.ReadFile(path)
	if len(raw) != lifecycle.LockPidWidth {
		t.Fatalf("content %q, want width %d", raw, lifecycle.LockPidWidth)
	}
	if pid, held := lifecycle.LockHeldBy(dir); !held || pid != os.Getpid() {
		t.Fatalf("LockHeldBy got %d %v", pid, held)
	}
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

// An older gx wrote the pid without padding; a new gx must still name it.
func TestAcquireLock_UnpaddedPidNamesTheHolder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, lifecycle.LockFileName)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("999999"); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = lifecycle.AcquireLock(path)
	if err == nil || !strings.Contains(err.Error(), "already running (pid 999999)") {
		t.Fatalf("got %v, want already running (pid 999999)", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("unpadded pid should fail at once, took %v", d)
	}
	if pid, held := lifecycle.LockHeldBy(dir); !held || pid != 999999 {
		t.Fatalf("LockHeldBy: got pid %d held %v, want 999999 true", pid, held)
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
