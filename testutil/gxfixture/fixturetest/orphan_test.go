package fixturetest

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
)

// A child the fixture started must not outlive Kill, even when the fixture
// itself is already gone.
func TestFixtureKill_LeavesNoOrphan(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := filepath.Join(dir, "fakefixture")
	body := "#!/bin/sh\nsleep 300 >/dev/null 2>&1 &\necho $! > " + pidFile + "\necho '{}'\nwait\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	old := FixtureBin
	FixtureBin = script
	t.Cleanup(func() { FixtureBin = old })

	var kill func()
	r, err := launchProc(func(f func()) { kill = f }, "ignored", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	child, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL) })

	// The leader dies without cleaning up, as when the fixture binary is killed.
	_ = r.cmd.Process.Signal(syscall.SIGKILL)
	kill()

	testutil.WaitFor(t, 10*time.Second, "the fixture's child to exit after Kill", func() bool { return !pidAlive(child) })
}
