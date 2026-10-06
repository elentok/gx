package storecommit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

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
		time.Sleep(50 * time.Millisecond)
		if got := logLines(t, dir); len(got) != 0 {
			t.Fatalf("after edit %d committed early: %v", i, got)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(logLines(t, dir)) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	got := logLines(t, dir)
	if len(got) != 1 || got[0] != "sync: 3 files" {
		t.Fatalf("log = %v, want one 'sync: 3 files'", got)
	}
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
	if err := EnsureRepo(dir); err != nil {
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
	if err := Immediate(filepath.Join(dir, "a.md"), "park"); err != nil {
		t.Fatal(err)
	}
	l.Stop()
	out, err := git(remote, "log", "--format=%s", "HEAD")
	if err != nil || strings.TrimSpace(out) != "park" {
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
	if err := Immediate(filepath.Join(dir, "a.md"), "park"); err != nil {
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
