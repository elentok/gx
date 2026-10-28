package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeGo puts a `go` shell script with the given body first on PATH.
func fakeGo(t *testing.T, body string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	prependPath(t, bin)
}

func briefDir(data string) string { return filepath.Join(data, "gx", "brief") }

func indexLines(t *testing.T, data string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(briefDir(data), "index.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
}

func TestBrief_SweepRemovesOldLogsAndIndexLines(t *testing.T) {
	data := envValue(envFor(t), "XDG_DATA_HOME")
	dir := briefDir(data)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldLog := filepath.Join(dir, "old.log")
	newLog := filepath.Join(dir, "new.log")
	for _, p := range []string{oldLog, newLog} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-15 * 24 * time.Hour)
	if err := os.Chtimes(oldLog, past, past); err != nil {
		t.Fatal(err)
	}
	oldLine := `{"time":"` + past.Format(time.RFC3339) + `","tool":"go-test","log":"old.log"}`
	newLine := `{"time":"` + time.Now().Add(-time.Hour).Format(time.RFC3339) + `","tool":"go-test","log":"new.log"}`
	if err := os.WriteFile(filepath.Join(dir, "index.jsonl"), []byte(oldLine+"\n"+newLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, code := runBriefCmd(t, goModule(t), "go-test", "./..."); code != 0 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}

	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Errorf("old log still exists: %v", err)
	}
	if _, err := os.Stat(newLog); err != nil {
		t.Errorf("new log removed: %v", err)
	}
	all := strings.Join(indexLines(t, data), "\n")
	if strings.Contains(all, "old.log") || !strings.Contains(all, "new.log") {
		t.Errorf("index after sweep:\n%s", all)
	}
	if n := len(indexLines(t, data)); n != 2 { // surviving line + this run
		t.Errorf("index has %d lines, want 2", n)
	}
}

func TestBrief_SweepErrorDoesNotFailRun(t *testing.T) {
	data := envValue(envFor(t), "XDG_DATA_HOME")
	dir := briefDir(data)
	// A directory named like a log, an unparsable index line and an index
	// that can only be rewritten through an unwritable temp path.
	if err := os.MkdirAll(filepath.Join(dir, "odd.log"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "index.jsonl.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-15 * 24 * time.Hour)
	oldLine := `{"time":"` + past.Format(time.RFC3339) + `","tool":"go-test"}`
	if err := os.WriteFile(filepath.Join(dir, "index.jsonl"), []byte("not json\n"+oldLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, code := runBriefCmd(t, goModule(t), "go-test", "./..."); code != 0 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
}

func TestBrief_SignalKilledChildIsFailure(t *testing.T) {
	data := envValue(envFor(t), "XDG_DATA_HOME")
	fakeGo(t, "echo partial-output\nkill -9 $$")

	out, code := runBriefCmd(t, t.TempDir(), "go-test")
	if code != 1 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	raw, err := os.ReadFile(lastLine(out))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "partial-output") {
		t.Errorf("log lost partial output: %q", raw)
	}
	if lines := indexLines(t, data); len(lines) != 1 || !strings.Contains(lines[0], `"exit":1`) {
		t.Errorf("index = %v", lines)
	}
}

func TestBrief_InterruptIsForwardedToChild(t *testing.T) {
	data := envValue(envFor(t), "XDG_DATA_HOME")
	started := filepath.Join(t.TempDir(), "started")
	fakeGo(t, "trap 'echo got-interrupt; exit 3' INT\necho partial-output\ntouch "+started+"\nwhile :; do sleep 0.05; done")

	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(started); err == nil {
				_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	out, code := runBriefCmd(t, t.TempDir(), "go-test")
	if code != 1 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	raw, err := os.ReadFile(lastLine(out))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "partial-output") || !strings.Contains(string(raw), "got-interrupt") {
		t.Errorf("log = %q", raw)
	}
	if len(indexLines(t, data)) != 1 {
		t.Errorf("want one index line")
	}
}
