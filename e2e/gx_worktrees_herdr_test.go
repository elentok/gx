// Package e2e drives the real `gx` binary inside a real `herdr` pane
// (via testutil/herdrctl), rather than the bubbletea model in-process
// (testutil/teatestv2). It exists to catch bugs that only show up at the
// process/terminal boundary — real herdr rendering, real pty timing, real
// process lifecycle — including bugs in herdr itself, which an in-process or
// faked-herdr test can't reach by construction.
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrctl"
)

var (
	buildGxOnce sync.Once
	gxBinPath   string
	buildGxErr  error
)

// gxBinary builds the real gx binary once per test run and returns its path.
func gxBinary(t *testing.T) string {
	t.Helper()
	buildGxOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gx-e2e-bin")
		if err != nil {
			buildGxErr = err
			return
		}
		gxBinPath = filepath.Join(dir, "gx")
		cmd := exec.Command("go", "build", "-o", gxBinPath, "github.com/elentok/gx")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildGxErr = err
			t.Logf("go build output:\n%s", out)
		}
	})
	if buildGxErr != nil {
		t.Fatalf("herdrctl e2e: build gx binary: %v", buildGxErr)
	}
	return gxBinPath
}

// TestWorktreesTab_LaunchesAndListsWorktree drives the real `gx worktrees`
// TUI inside a real herdr pane end to end: launch the binary, wait for its
// first real-terminal frame, and assert the seeded worktree is rendered.
// It's a small smoke scenario proving the herdr-backed harness works, not a
// specific bug repro.
func TestWorktreesTab_LaunchesAndListsWorktree(t *testing.T) {
	herdrctl.RequireHerdr(t)

	repoDir, _ := testutil.TempRepoWithLinkedWorktree(t, "feature-x")

	bin := gxBinary(t)

	ws := herdrctl.NewWorkspace(t, repoDir)
	ws.Run(bin, "worktrees")

	ws.WaitForText("feature-x", 15*time.Second)
}
