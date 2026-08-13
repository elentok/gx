// Package herdrctl drives a real `herdr` daemon from Go tests, over the same
// CLI/socket API `ralphloop` uses in production. Unlike testutil/herdrfake
// (which fakes the herdr process boundary), this package execs the real
// `herdr` binary — it exists specifically to catch bugs that live in herdr
// itself, which a fake can't reproduce by construction.
//
// Each test gets its own workspace (herdr's isolation unit), so tests can
// share the same herdr instance safely. On failure, the workspace is left
// open instead of closed, so it can be inspected live rather than guessed at
// from a log — mirroring the tui-testing-with-herdr skill's failure protocol.
package herdrctl

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// RequireHerdr skips the test unless a real herdr daemon is reachable: herdr
// must be on PATH, and HERDR_ENV=1 must be set (herdr's own gate on its
// socket API being available in the current terminal session).
func RequireHerdr(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("herdr"); err != nil {
		t.Skip("herdrctl: herdr not found on PATH")
	}
	if os.Getenv("HERDR_ENV") != "1" {
		t.Skip("herdrctl: HERDR_ENV=1 not set, no herdr daemon reachable")
	}
}

// Workspace is an isolated herdr workspace with a single root pane, torn down
// (or left open for inspection) at the end of the test.
type Workspace struct {
	t          *testing.T
	ID         string
	RootPaneID string
}

// NewWorkspace creates a fresh herdr workspace rooted at cwd, labeled after
// the test, and registers cleanup: the workspace is closed if the test
// passed, and left open (with its IDs logged) if the test failed, so a
// failure can be inspected live.
func NewWorkspace(t *testing.T, cwd string) *Workspace {
	t.Helper()

	out := run(t, "workspace", "create", "--cwd", cwd, "--label", "gx-e2e-"+t.Name(), "--no-focus")
	var resp struct {
		Result struct {
			RootPane struct {
				PaneID string `json:"pane_id"`
			} `json:"root_pane"`
			Workspace struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"workspace"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("herdrctl: parse workspace create response: %v\nraw: %s", err, out)
	}

	w := &Workspace{
		t:          t,
		ID:         resp.Result.Workspace.WorkspaceID,
		RootPaneID: resp.Result.RootPane.PaneID,
	}

	t.Cleanup(func() {
		if t.Failed() {
			visible, _ := tryRead(w.RootPaneID)
			t.Logf("herdrctl: test failed, leaving workspace %s open for inspection (root pane %s)\nlast visible output:\n%s",
				w.ID, w.RootPaneID, visible)
			return
		}
		run(t, "workspace", "close", w.ID)
	})

	return w
}

// Run launches command in the workspace's root pane (types it and presses
// Enter, same as a user running it interactively).
func (w *Workspace) Run(command ...string) {
	w.t.Helper()
	run(w.t, append([]string{"pane", "run", w.RootPaneID}, command...)...)
}

// SendText types literal text into the root pane without pressing Enter.
func (w *Workspace) SendText(text string) {
	w.t.Helper()
	run(w.t, "pane", "send-text", w.RootPaneID, text)
}

// SendKeys sends one or more named key presses (e.g. "Enter", "Tab",
// "ArrowDown", "ctrl+c", "esc") to the root pane.
func (w *Workspace) SendKeys(keys ...string) {
	w.t.Helper()
	run(w.t, append([]string{"pane", "send-keys", w.RootPaneID}, keys...)...)
}

// WaitForText polls the root pane's rendered (alternate-screen-aware) output
// until it contains want, failing the test if timeout elapses first.
func (w *Workspace) WaitForText(want string, timeout time.Duration) {
	w.t.Helper()
	w.waitOutput("--match", want, timeout)
}

// WaitForRegex polls the root pane's rendered output until it matches
// pattern (a Rust regex), failing the test if timeout elapses first.
func (w *Workspace) WaitForRegex(pattern string, timeout time.Duration) {
	w.t.Helper()
	w.waitOutput("--regex", pattern, timeout)
}

func (w *Workspace) waitOutput(flag, value string, timeout time.Duration) {
	w.t.Helper()
	cmd := exec.Command("herdr", "pane", "wait-output", w.RootPaneID,
		flag, value, "--source", "visible", "--timeout", strconv.FormatInt(timeout.Milliseconds(), 10))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		visible, _ := tryRead(w.RootPaneID)
		w.t.Fatalf("herdrctl: wait-output %s=%q timed out after %s: %s\ncurrent visible output:\n%s",
			flag, value, timeout, stdout.String(), visible)
	}
}

// Read returns the root pane's currently rendered ("visible") screen.
// Unlike the other herdr subcommands this package wraps, `pane read` prints
// its snapshot as raw text on stdout rather than a JSON envelope.
func (w *Workspace) Read() string {
	w.t.Helper()
	return string(run(w.t, "pane", "read", w.RootPaneID, "--source", "visible"))
}

func run(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("herdr", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("herdrctl: herdr %v: %v\nstdout: %s\nstderr: %s", args, err, stdout.String(), stderr.String())
	}
	return stdout.Bytes()
}

// tryRead best-effort reads a pane's visible output without failing the
// test, for use inside cleanup/error paths where the pane may already be
// gone.
func tryRead(paneID string) (string, error) {
	cmd := exec.Command("herdr", "pane", "read", paneID, "--source", "visible")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
