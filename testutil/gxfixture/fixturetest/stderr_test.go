package fixturetest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
)

// useScript points FixtureBin at a shell script for the test. These tests must
// not run in parallel: FixtureBin is a package variable.
func useScript(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := FixtureBin
	FixtureBin = path
	t.Cleanup(func() { FixtureBin = old })
}

func TestStderr_StartErrorShowsTheCause(t *testing.T) {
	useScript(t, "echo boom >&2\nexit 1")
	_, err := LaunchProc(func(func()) {}, "demo", nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want it to contain the child's stderr", err)
	}
}

func TestStderr_ChildrenDoNotMix(t *testing.T) {
	launch := func(marker string) *Running {
		useScript(t, fmt.Sprintf("echo %s >&2\necho '{}'\nexec sleep 30", marker))
		r, err := LaunchProc(t.Cleanup, "demo", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Poll(5*time.Second, 10*time.Millisecond, func() bool { return strings.Contains(r.Stderr(), marker) }) {
			t.Fatalf("stderr = %q, want %s", r.Stderr(), marker)
		}
		return r
	}
	a, b := launch("marker-a"), launch("marker-b")
	if got := a.Stderr(); strings.Contains(got, "marker-b") {
		t.Errorf("a's stderr holds b's marker: %q", got)
	}
	if got := b.Stderr(); strings.Contains(got, "marker-a") {
		t.Errorf("b's stderr holds a's marker: %q", got)
	}
}

type recorder struct {
	failed bool
	logs   []string
}

func (r *recorder) Failed() bool { return r.failed }
func (r *recorder) Logf(format string, args ...any) {
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

func TestStderr_LoggedOnlyWhenTheTestFailed(t *testing.T) {
	r := &Running{stderr: &syncBuffer{}}
	_, _ = r.stderr.Write([]byte("cause\n"))
	for _, failed := range []bool{false, true} {
		rec := &recorder{failed: failed}
		r.logStderrIfFailed(rec)
		if got := len(rec.logs) == 1 && strings.Contains(rec.logs[0], "cause"); got != failed {
			t.Errorf("failed=%v: logs = %q", failed, rec.logs)
		}
	}
}

func TestStderr_TailKeepsTheLastLines(t *testing.T) {
	r := &Running{stderr: &syncBuffer{}}
	_, _ = r.stderr.Write([]byte("one\ntwo\nthree\n"))
	if got := r.StderrTail(2); got != "two\nthree" {
		t.Errorf("tail = %q", got)
	}
}
