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
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
)

var ensureActiveWorkspaceOnce sync.Once

// ensureActiveWorkspace guarantees herdr's server already has an active
// (focused) workspace before any test workspace is created with
// --no-focus. herdr's create_workspace_with_launch_env focuses a new
// workspace whenever the server has never had an active workspace yet,
// regardless of --no-focus — on a freshly started server (as on CI, unlike
// a real dev machine's herdr, which already has one from live use) this
// silently promotes the first test's --no-focus workspace to active,
// which then makes herdr report its idle-completion as "idle" instead of
// "done" (busy-pane classification suppresses the notification/done state
// for the focused tab). A throwaway sentinel workspace, focused once up
// front, keeps every real test workspace correctly out of focus.
func ensureActiveWorkspace(t *testing.T) {
	t.Helper()
	ensureActiveWorkspaceOnce.Do(func() {
		run(t, "workspace", "create", "--cwd", os.TempDir(), "--label", "gx-e2e-sentinel", "--focus")
	})
}

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
	ensureActiveWorkspace(t)

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
// alone isn't enough for this: the pane's interactive shell re-derives PATH
// from its own config on startup and can reorder an inherited entry behind
// its own configured dirs. The pane's shell varies by environment (fish
// locally, bash on GitHub's macOS runners), so this detects it via `herdr
// pane process-info` and sends shell-appropriate syntax.
//
// It then runs a uniquely-tagged `echo` and waits for that echo's own
// output, rather than returning as soon as the PATH command is sent: on a
// loaded CI runner, herdr's own bookkeeping of "is this pane an available
// shell" lags slightly behind the shell actually finishing the prior
// command, and an AgentStart issued immediately after can race that lag
// (agent_pane_busy, or a stale idle/done read later on). Waiting for our
// own echo to come back proves the shell has fully round-tripped the PATH
// command before anything else touches the pane.
func (w *Workspace) PrependPath(dir string) {
	w.t.Helper()
	if w.shellName() == "fish" {
		w.Run("fish_add_path", "--prepend", "--move", dir)
	} else {
		w.Run("export", "PATH="+dir+":$PATH")
	}

	// Split the sentinel across two quoted printf args: `pane run` types the
	// command verbatim (visible in the pane before it even executes), so a
	// sentinel passed whole to `echo` would match on the typed input, not
	// its output. Split like this, the full sentinel is contiguous only in
	// printf's concatenated *output* — never in the input line, where the
	// quotes and space keep the two halves apart.
	sentinel := fmt.Sprintf("herdrctl-prependpath-ready-%s-%d", w.RootPaneID, time.Now().UnixNano())
	half := len(sentinel) / 2
	w.Run("printf", "'%s%s'", "'"+sentinel[:half]+"'", "'"+sentinel[half:]+"'")
	w.WaitForText(sentinel, 10*time.Second)
}

// shellName returns the root pane's foreground shell process name (e.g.
// "fish", "bash", "zsh"), read via `herdr pane process-info`.
func (w *Workspace) shellName() string {
	w.t.Helper()
	out := run(w.t, "pane", "process-info", "--pane", w.RootPaneID)
	var resp struct {
		Result struct {
			ProcessInfo struct {
				ForegroundProcesses []struct {
					Name string `json:"name"`
				} `json:"foreground_processes"`
			} `json:"process_info"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		w.t.Fatalf("herdrctl: parse process-info response: %v\nraw: %s", err, out)
	}
	if len(resp.Result.ProcessInfo.ForegroundProcesses) == 0 {
		w.t.Fatalf("herdrctl: process-info returned no foreground processes\nraw: %s", out)
	}
	return resp.Result.ProcessInfo.ForegroundProcesses[0].Name
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

// AgentGet reads an agent's current status without waiting for a transition
// or submitting a prompt, via `herdr agent get`. target defaults to the
// workspace's root pane if empty.
func (w *Workspace) AgentGet(target string) herdr.Agent {
	w.t.Helper()
	if target == "" {
		target = w.RootPaneID
	}
	agent, err := herdr.AgentGet(target)
	if err != nil {
		w.t.Fatalf("herdrctl: agent get: %v", err)
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
