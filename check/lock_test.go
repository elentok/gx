package check

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLockHolderProcess is the child of TestFullLock_KilledHolderReleases: it
// takes the lock, says so and waits to be killed.
func TestLockHolderProcess(t *testing.T) {
	dir := os.Getenv("GX_TEST_LOCK_HOLD")
	if dir == "" {
		t.Skip("child process only")
	}
	if _, err := AcquireFullLock(dir, "", nil); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "held"), nil, 0o644)
	time.Sleep(time.Minute)
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestFullLock_KilledHolderReleases(t *testing.T) {
	dir := t.TempDir()
	child := exec.Command(os.Args[0], "-test.run=^TestLockHolderProcess$")
	child.Env = append(os.Environ(), "GX_TEST_LOCK_HOLD="+dir)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer child.Process.Kill()
	waitFor(t, "child to hold the lock", func() bool { _, err := os.Stat(filepath.Join(dir, "held")); return err == nil })

	got := make(chan *FullLock)
	go func() {
		l, err := AcquireFullLock(dir, "/wt", nil)
		if err != nil {
			t.Error(err)
		}
		got <- l
	}()
	waitFor(t, "a waiting file", func() bool { return len(ReadWaiting(dir)) == 1 })
	if w := ReadWaiting(dir)[0]; w.PID != os.Getpid() {
		t.Errorf("waiting pid = %d, want %d", w.PID, os.Getpid())
	} else if w.Root != "/wt" {
		t.Errorf("waiting root = %q, want /wt", w.Root)
	}
	select {
	case <-got:
		t.Fatal("lock taken while the child holds it")
	case <-time.After(100 * time.Millisecond):
	}
	child.Process.Kill()
	select {
	case l := <-got:
		defer l.Release()
		if l.Waited <= 0 {
			t.Error("Waited not set")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("lock not released by the killed holder")
	}
	if w := ReadWaiting(dir); len(w) != 0 {
		t.Errorf("waiting state left after the lock was taken: %+v", w)
	}
}

func TestFullLock_WaitLineNamesHolderAndClears(t *testing.T) {
	dir := t.TempDir()
	first, err := AcquireFullLock(dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	first.SetRun("RUN-A")
	var out syncBuf
	got := make(chan *FullLock)
	go func() {
		l, _ := AcquireFullLock(dir, "", &out)
		got <- l
	}()
	waitFor(t, "the wait line", func() bool { return strings.Contains(out.String(), "RUN-A") })
	if w := ReadWaiting(dir); len(w) != 1 || w[0].HeldBy != "RUN-A" {
		t.Errorf("waiting = %+v, want one held by RUN-A", w)
	}
	first.Release()
	l := <-got
	defer l.Release()
	if !strings.HasSuffix(out.String(), "\r\x1b[K") {
		t.Errorf("line not cleared: %q", out.String())
	}
}

func TestReadWaiting_DeadPidIsNotAccepted(t *testing.T) {
	dir := t.TempDir()
	p := exec.Command("true")
	if err := p.Run(); err != nil {
		t.Fatal(err)
	}
	wdir := filepath.Join(dir, checkWaitingDir)
	os.MkdirAll(wdir, 0o755)
	data, _ := json.Marshal(Waiting{PID: p.Process.Pid})
	os.WriteFile(filepath.Join(wdir, strconv.Itoa(p.Process.Pid)+".json"), data, 0o644)
	if w := ReadWaiting(dir); len(w) != 0 {
		t.Errorf("dead waiter accepted: %+v", w)
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }
