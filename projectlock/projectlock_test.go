package projectlock

import (
	"bufio"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/config"
)

func makeProject(t *testing.T) (projectDir, ticketPath string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	projectDir = filepath.Join(t.TempDir(), "proj")
	issues := filepath.Join(projectDir, "epic", "issues")
	if err := os.MkdirAll(issues, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, projectFile), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return projectDir, filepath.Join(issues, "01-a.md")
}

// blocked reports whether fn is still waiting after d.
func blocked(d time.Duration, fn func()) (isBlocked bool, done <-chan struct{}) {
	ch := make(chan struct{})
	go func() { fn(); close(ch) }()
	select {
	case <-ch:
		return false, ch
	case <-time.After(d):
		return true, ch
	}
}

func TestExclusiveBlocksShared(t *testing.T) {
	dir, tp := makeProject(t)
	unlockEx, err := Exclusive(dir)
	if err != nil {
		t.Fatal(err)
	}
	isBlocked, done := blocked(100*time.Millisecond, func() {
		u, err := Shared(tp)
		if err == nil {
			u()
		}
	})
	if !isBlocked {
		t.Fatal("shared lock was granted while exclusive was held")
	}
	unlockEx()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shared lock did not proceed after release")
	}
}

func TestSharedDoesNotBlockShared(t *testing.T) {
	_, tp := makeProject(t)
	u1, err := Shared(tp)
	if err != nil {
		t.Fatal(err)
	}
	defer u1()
	isBlocked, _ := blocked(500*time.Millisecond, func() {
		u2, err := Shared(tp)
		if err == nil {
			u2()
		}
	})
	if isBlocked {
		t.Fatal("second shared lock blocked")
	}
}

func TestLockFileStaysOutOfProjectDir(t *testing.T) {
	dir, tp := makeProject(t)
	u, err := Shared(tp)
	if err != nil {
		t.Fatal(err)
	}
	u()
	lp, err := lockPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := filepath.Rel(dir, lp); err == nil && !strings.HasPrefix(rel, "..") {
		t.Fatalf("lock file %s is inside the project dir", lp)
	}
	var found []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, _ error) error {
		if strings.HasSuffix(p, ".lock") {
			found = append(found, p)
		}
		return nil
	})
	if len(found) > 0 {
		t.Fatalf("lock files in project dir: %v", found)
	}
}

func TestLockPathIgnoresTMPDIR(t *testing.T) {
	dir, _ := makeProject(t)
	t.Setenv("TMPDIR", t.TempDir())
	a, err := lockPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", t.TempDir())
	b, err := lockPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("lock path follows TMPDIR: %s vs %s", a, b)
	}
	state, err := config.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(a) != state {
		t.Fatalf("lock %s is not directly under state dir %s", a, state)
	}
}

func TestLockPathStateDirErrorIsReturned(t *testing.T) {
	dir, _ := makeProject(t)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", blocker)
	if _, err := Exclusive(dir); err == nil {
		t.Fatal("expected an error when the state dir cannot be created")
	}
}

func TestOtherProjectDoesNotWait(t *testing.T) {
	dirA, _ := makeProject(t)
	_, tpB := makeProject(t)
	u, err := Exclusive(dirA)
	if err != nil {
		t.Fatal(err)
	}
	defer u()
	isBlocked, _ := blocked(500*time.Millisecond, func() {
		u2, err := Shared(tpB)
		if err == nil {
			u2()
		}
	})
	if isBlocked {
		t.Fatal("write to another project waited")
	}
}

func TestPathOutsideStoreTakesNoLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".scratch", "epic", "issues", "01-a.md")
	if _, ok := ProjectOf(p); ok {
		t.Fatal("old-tree path resolved to a project")
	}
	u, err := Shared(p)
	if err != nil {
		t.Fatal(err)
	}
	u()
}

func TestWaitHookFiresOnce(t *testing.T) {
	dir, tp := makeProject(t)
	defer SetWaitHookDelayForTest(50 * time.Millisecond)()
	var calls atomic.Int32
	var got atomic.Value
	defer SetWaitHook(func(project string) { calls.Add(1); got.Store(project) })()

	unlockEx, err := Exclusive(dir)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		u, err := Shared(tp)
		if err == nil {
			u()
		}
		close(done)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	unlockEx()
	<-done
	if n := calls.Load(); n != 1 {
		t.Fatalf("hook calls = %d, want 1", n)
	}
	if got.Load() != "proj" {
		t.Fatalf("hook project = %v, want proj", got.Load())
	}
}

// TestHelperHoldExclusive is the child process: it takes the lock, says so,
// then waits to be killed.
func TestHelperHoldExclusive(t *testing.T) {
	dir := os.Getenv("PROJECTLOCK_HOLD")
	if dir == "" {
		t.Skip("child helper")
	}
	if _, err := Exclusive(dir); err != nil {
		t.Fatal(err)
	}
	os.Stdout.WriteString("locked\n")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

func TestExclusiveReleasesWhenHolderDies(t *testing.T) {
	dir, tp := makeProject(t)
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperHoldExclusive$")
	cmd.Env = append(os.Environ(), "PROJECTLOCK_HOLD="+dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if sc.Text() == "locked" {
			break
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	isBlocked, done := blocked(100*time.Millisecond, func() {
		u, err := Shared(tp)
		if err == nil {
			u()
		}
	})
	if !isBlocked {
		t.Fatal("shared lock granted while child held exclusive")
	}
	cmd.Process.Kill()
	cmd.Wait()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("lock not released after holder died")
	}
}
