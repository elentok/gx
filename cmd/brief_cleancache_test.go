package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/gocachegate"
	"github.com/elentok/gx/testutil"
)

// fakeGoRecording puts a `go` on PATH that appends its arguments to the
// returned file.
func fakeGoRecording(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	rec := filepath.Join(t.TempDir(), "calls")
	testutil.WriteFile(t, bin, "go", "#!/bin/sh\necho \"$*\" >> '"+rec+"'\n")
	if err := os.Chmod(filepath.Join(bin, "go"), 0o755); err != nil {
		t.Fatal(err)
	}
	prependPath(t, bin)
	return rec
}

func runCleanCache(t *testing.T, gate gocachegate.Deps) (string, int) {
	t.Helper()
	root := newRootCmd(deps{getwd: func() (string, error) { return t.TempDir(), nil }, env: envFor(t), cacheGate: gate})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"brief", "go-clean-cache"})
	err := root.Execute()
	var ee *ExitError
	if errors.As(err, &ee) {
		return out.String(), ee.Code
	}
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	return out.String(), 0
}

func quietGate(capacity time.Duration) gocachegate.Deps {
	return gocachegate.Deps{Cap: capacity, Poll: 10 * time.Millisecond,
		Scan: func() (bool, error) { return false, nil }, Logf: func(string, ...any) {}}
}

func TestBrief_GoCleanCacheRunsCleanWithoutGateHolder(t *testing.T) {
	rec := fakeGoRecording(t)
	out, code := runCleanCache(t, quietGate(time.Second))
	if code != 0 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	got, err := os.ReadFile(rec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "clean -cache" {
		t.Errorf("go calls = %q", got)
	}
}

func TestBrief_GoCleanCacheWaitsForRegisteredBuild(t *testing.T) {
	rec := fakeGoRecording(t)
	stateDir, err := config.DirsFrom(envFor(t)).StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	releaseBuild := gocachegate.Register(stateDir)

	type result struct {
		out  string
		code int
	}
	done := make(chan result, 1)
	go func() {
		out, code := runCleanCache(t, quietGate(10*time.Second))
		done <- result{out, code}
	}()

	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(rec); err == nil {
		t.Fatal("go clean ran while a build held the gate")
	}
	select {
	case r := <-done:
		t.Fatalf("returned while a build held the gate: %d %s", r.code, r.out)
	default:
	}

	releaseBuild()
	select {
	case r := <-done:
		if r.code != 0 {
			t.Fatalf("exit = %d, out:\n%s", r.code, r.out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("did not finish after the build released the gate")
	}
	if _, err := os.Stat(rec); err != nil {
		t.Errorf("go clean never ran: %v", err)
	}
}

func TestBrief_GoCleanCacheFailureExits1(t *testing.T) {
	bin := t.TempDir()
	testutil.WriteFile(t, bin, "go", "#!/bin/sh\necho boom\nexit 3\n")
	if err := os.Chmod(filepath.Join(bin, "go"), 0o755); err != nil {
		t.Fatal(err)
	}
	prependPath(t, bin)
	out, code := runCleanCache(t, quietGate(time.Second))
	if code != 1 || !strings.Contains(out, "boom") {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
}

func TestBrief_HelpListsGoCleanCache(t *testing.T) {
	out, code := runBriefCmd(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "go-clean-cache") {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
}

// Agent docs name `go clean -cache` only in the rule that forbids it.
func TestAgentDocs_GoCleanCacheOnlyInRule(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skipf("not in a git checkout: %v", err)
	}
	dir := strings.TrimSpace(string(root))
	var hits []string
	for _, f := range []string{"AGENTS.md", "CLAUDE.md", filepath.Join(".ai", "index.md")} {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, "go clean -cache") {
				hits = append(hits, f+": "+line)
			}
		}
	}
	want := ".ai/index.md: - Never run `go clean -cache` directly, use `gx brief go-clean-cache`."
	if len(hits) != 1 || hits[0] != want {
		t.Errorf("hits = %q, want only %q", hits, want)
	}
}
