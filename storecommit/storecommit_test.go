package storecommit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitpkg "github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func git(dir string, args ...string) (string, error) { return runGit(dir, nil, args...) }

func logLines(t *testing.T, dir string) []string {
	t.Helper()
	out, err := git(dir, "log", "--format=%s")
	if err != nil {
		return nil
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func waitForCommit(t *testing.T, dir string) {
	t.Helper()
	testutil.WaitFor(t, 5*time.Second, "a commit", func() bool { return len(logLines(t, dir)) > 0 })
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// quiet is the one place a test waits out a window to prove nothing happened.
func quiet(d time.Duration) { time.Sleep(d) }

func TestStart_CreatesRepoIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	l, err := Start(dir, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Stop()
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("store is not a git repo: %v", err)
	}
}

func TestEditsInsideDebounceWindowMakeOneSyncCommit(t *testing.T) {
	dir := t.TempDir()
	l, err := start(dir, 300*time.Millisecond, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Stop()

	for i, name := range []string{"a.md", "b.md", "c.md"} {
		write(t, dir, name, "x")
		quiet(50 * time.Millisecond)
		if got := logLines(t, dir); len(got) != 0 {
			t.Fatalf("after edit %d committed early: %v", i, got)
		}
	}
	waitForCommit(t, dir)
	got := logLines(t, dir)
	if len(got) != 1 || got[0] != "sync: 3 files" {
		t.Fatalf("log = %v, want one 'sync: 3 files'", got)
	}
}

func TestTouch_CommitsAfterQuietWithoutPolling(t *testing.T) {
	dir := t.TempDir()
	l, err := start(dir, 200*time.Millisecond, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Stop()

	write(t, dir, "a.md", "x")
	l.Touch()
	quiet(100 * time.Millisecond)
	write(t, dir, "b.md", "x")
	l.Touch() // restarts the quiet period
	quiet(150 * time.Millisecond)
	if got := logLines(t, dir); len(got) != 0 {
		t.Fatalf("committed before the quiet period ended: %v", got)
	}
	waitForCommit(t, dir)
	if got := logLines(t, dir); len(got) != 1 || got[0] != "sync: 2 files" {
		t.Fatalf("log = %v, want one 'sync: 2 files'", got)
	}
}

func TestCheck_CommitsUnreportedEditAfterQuiet(t *testing.T) {
	dir := t.TempDir()
	l, err := start(dir, 20*time.Millisecond, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Stop()

	write(t, dir, "a.md", "x")
	l.Check()
	waitForCommit(t, dir)
	if got := logLines(t, dir); len(got) != 1 {
		t.Fatalf("log = %v, want one commit", got)
	}
}

func TestCheck_RepeatedChecksDoNotPostponeCommit(t *testing.T) {
	dir := t.TempDir()
	l, err := start(dir, 200*time.Millisecond, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Stop()

	write(t, dir, "a.md", "x")
	for range 5 {
		l.Check()
		quiet(30 * time.Millisecond)
	}
	waitForCommit(t, dir)
	if got := logLines(t, dir); len(got) != 1 {
		t.Fatalf("log = %v, want one commit", got)
	}
}

func TestNoTouchNoFallback_RunsNoGit(t *testing.T) {
	dir := t.TempDir()
	l, err := start(dir, 20*time.Millisecond, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "a.md", "x")
	quiet(200 * time.Millisecond)
	if got := logLines(t, dir); len(got) != 0 {
		t.Fatalf("committed with no touch and no fallback tick: %v", got)
	}
	l.Stop()
}

func TestImmediate_CommitsWithMessageInsideStore(t *testing.T) {
	dir := t.TempDir()
	l, err := start(dir, time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Stop()
	write(t, dir, "t.md", "x")
	if err := Immediate(filepath.Join(dir, "epic", "issues", "t.md"), "epic/32: done"); err != nil {
		t.Fatal(err)
	}
	if got := logLines(t, dir); len(got) != 1 || got[0] != "epic/32: done" {
		t.Fatalf("log = %v", got)
	}
}

func TestImmediate_NoLoopIsNoop(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureRepo(dir, nil); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "t.md", "x")
	if err := Immediate(filepath.Join(dir, "t.md"), "m"); err != nil {
		t.Fatal(err)
	}
	if got := logLines(t, dir); len(got) != 0 {
		t.Fatalf("committed without a loop: %v", got)
	}
}

func TestOptionsEnv_ReachesEveryGitChild(t *testing.T) {
	remote := t.TempDir()
	if _, err := git(remote, "init", "-q", "--bare"); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(t.TempDir(), "trace2.json")
	env := append(gitpkg.NonInteractiveEnv(), "GIT_TRACE2_EVENT="+trace)
	dir := t.TempDir()
	l, err := StartWith(dir, Options{Debounce: time.Hour, PushRemote: remote, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "a.md", "x")
	if err := Immediate(filepath.Join(dir, "a.md"), "pause"); err != nil {
		t.Fatal(err)
	}
	l.Stop()
	raw, err := os.ReadFile(trace)
	if err != nil {
		t.Fatalf("trace file not written, env did not reach git: %v", err)
	}
	for _, cmd := range []string{"init", "add", "commit", "push"} {
		if !strings.Contains(string(raw), `"name":"`+cmd+`"`) {
			t.Errorf("no %s git child traced in:\n%s", cmd, raw)
		}
	}
}

func TestPush_PushesAfterCommit(t *testing.T) {
	remote := t.TempDir()
	if _, err := git(remote, "init", "-q", "--bare"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	l, err := StartWith(dir, Options{Debounce: time.Hour, PushRemote: remote})
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "a.md", "x")
	if err := Immediate(filepath.Join(dir, "a.md"), "pause"); err != nil {
		t.Fatal(err)
	}
	l.Stop()
	out, err := git(remote, "log", "--format=%s", "HEAD")
	if err != nil || strings.TrimSpace(out) != "pause" {
		t.Fatalf("remote log = %q, err %v", out, err)
	}
}

func TestSetSettings_NextCommitUsesNewDebounceAndRemote(t *testing.T) {
	remote := t.TempDir()
	if _, err := git(remote, "init", "-q", "--bare"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	l, err := StartWith(dir, Options{Debounce: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	l.SetSettings(100*time.Millisecond, remote)
	write(t, dir, "a.md", "x")
	l.Touch()
	waitForCommit(t, dir)
	if got := logLines(t, dir); len(got) != 1 || got[0] != "sync: 1 files" {
		t.Fatalf("log = %v, want one debounced sync commit", got)
	}
	l.Stop()
	if out, err := git(remote, "log", "--format=%s", "HEAD"); err != nil || strings.TrimSpace(out) != "sync: 1 files" {
		t.Fatalf("remote log = %q, err %v", out, err)
	}
}

func TestPush_FailureIsReportedAndDoesNotBlockCommits(t *testing.T) {
	dir := t.TempDir()
	failed := make(chan error, 4)
	l, err := StartWith(dir, Options{Debounce: time.Hour, PushRemote: filepath.Join(t.TempDir(), "missing"),
		OnError: func(err error) { failed <- err }})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Stop()
	write(t, dir, "a.md", "x")
	if err := Immediate(filepath.Join(dir, "a.md"), "pause"); err != nil {
		t.Fatal(err)
	}
	if got := logLines(t, dir); len(got) != 1 {
		t.Fatalf("log = %v, want the commit despite the push", got)
	}
	select {
	case <-failed:
	case <-time.After(5 * time.Second):
		t.Fatal("push failure was not reported")
	}
}
