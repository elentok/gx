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

	"github.com/elentok/gx/herdr"
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
// failure can be inspected live. env entries (each "KEY=VALUE") are set on
// the launched process, e.g. to prepend a fake-agent binary's directory onto
// PATH so `herdr agent start --kind claude` finds it instead of a real
// claude.
func NewWorkspace(t *testing.T, cwd string, env ...string) *Workspace {
	t.Helper()

	args := []string{"workspace", "create", "--cwd", cwd, "--label", "gx-e2e-" + t.Name(), "--no-focus"}
	for _, e := range env {
		args = append(args, "--env", e)
	}
	out := run(t, args...)
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

// PrependPath puts dir at the very front of the root pane's shell PATH, for
// e.g. making a fake agent binary (testutil/agentfake) resolve ahead of a
// real one before calling AgentStart. herdr's `workspace create --env`
// alone isn't enough for this: the pane's interactive shell (fish, in this
// environment) re-derives PATH from its own config on startup and can
// reorder an inherited entry behind its own configured dirs, so this uses
// fish's own `fish_add_path --prepend --move` instead of relying on the
// process's initial environment.
func (w *Workspace) PrependPath(dir string) {
	w.t.Helper()
	w.Run("fish_add_path", "--prepend", "--move", dir)
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

// AgentStart starts an interactive agent (e.g. claude, or a fake standing in
// for one — see testutil/agentfake) in the workspace's root pane via `herdr
// agent start`. opts.Pane defaults to the workspace's root pane if unset.
func (w *Workspace) AgentStart(opts herdr.AgentStartOptions) herdr.Agent {
	w.t.Helper()
	if opts.Pane == "" {
		opts.Pane = w.RootPaneID
	}
	agent, err := herdr.AgentStart(opts)
	if err != nil {
		w.t.Fatalf("herdrctl: agent start: %v", err)
	}
	return agent
}

// AgentPrompt submits a prompt to an agent via `herdr agent prompt`.
// opts.Target defaults to the workspace's root pane if unset.
func (w *Workspace) AgentPrompt(opts herdr.AgentPromptOptions) herdr.Agent {
	w.t.Helper()
	if opts.Target == "" {
		opts.Target = w.RootPaneID
	}
	agent, err := herdr.AgentPrompt(opts)
	if err != nil {
		w.t.Fatalf("herdrctl: agent prompt: %v", err)
	}
	return agent
}

// AgentWait blocks until an agent reaches one of the requested states via
// `herdr agent wait`. opts.Target defaults to the workspace's root pane if
// unset.
func (w *Workspace) AgentWait(opts herdr.AgentWaitOptions) herdr.Agent {
	w.t.Helper()
	if opts.Target == "" {
		opts.Target = w.RootPaneID
	}
	agent, err := herdr.AgentWait(opts)
	if err != nil {
		w.t.Fatalf("herdrctl: agent wait: %v", err)
	}
	return agent
}

// AgentExplain reports which detection rule herdr's pane monitor matched for
// target's current state, via `herdr agent explain`. target defaults to the
// workspace's root pane if empty.
func (w *Workspace) AgentExplain(target string) herdr.AgentExplainResult {
	w.t.Helper()
	if target == "" {
		target = w.RootPaneID
	}
	result, err := herdr.AgentExplain(target)
	if err != nil {
		w.t.Fatalf("herdrctl: agent explain: %v", err)
	}
	return result
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
