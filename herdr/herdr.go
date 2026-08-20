// Package herdr wraps the herdr CLI's workspace/worktree/tab/agent socket-API
// commands as plain functions returning (result, error). It knows nothing
// about bubbletea, so both the worktrees UI (via tea.Cmd wrappers) and
// non-TUI callers like ralph-loop can shell out to herdr directly.
package herdr

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// runCommand is a seam so tests can fake `herdr` invocations without
// spawning the real executable.
var runCommand = func(args ...string) ([]byte, error) {
	return exec.Command("herdr", args...).CombinedOutput()
}

// AgentNameTakenError is herdr's agent_name_taken failure: the requested
// agent name is already in use by a live pane. CandidateCwd is the cwd of
// the first candidate herdr reported in Message, if one could be parsed —
// callers use it to tell "this is our own already-running launch" (the
// candidate's cwd matches the cwd we tried to launch in) from "some other,
// unrelated pane happens to share this name".
type AgentNameTakenError struct {
	Message      string
	CandidateCwd string
	wrapped      error
}

func (e *AgentNameTakenError) Error() string { return e.wrapped.Error() }
func (e *AgentNameTakenError) Unwrap() error { return e.wrapped }

// AgentBlockedError is herdr 0.8.2's agent_blocked failure: the target pane
// is sitting on an unanswered dialog and refused the command outright before
// touching the runtime. Message is herdr's own description, which names the
// pane a caller reports to an operator — callers must not paper over this
// with a "pane run" bypass (see the epic's ticket 01 findings) since a pane
// showing a dialog is not ready to receive a prompt at all.
type AgentBlockedError struct {
	Message string
	wrapped error
}

func (e *AgentBlockedError) Error() string { return e.wrapped.Error() }
func (e *AgentBlockedError) Unwrap() error { return e.wrapped }

// AgentNameLostError is herdr 0.8.2's agent_name_lost failure: the target
// pane's terminal identity changed between the launch request and the
// readiness check, so the pane gx asked to start an agent in is no longer
// the pane herdr resolved. Message is herdr's own description, which names
// the lost pane.
type AgentNameLostError struct {
	Message string
	wrapped error
}

func (e *AgentNameLostError) Error() string { return e.wrapped.Error() }
func (e *AgentNameLostError) Unwrap() error { return e.wrapped }

// AgentNotReadyError is herdr 0.8.2's agent_not_ready failure: `agent start`'s
// readiness poll returned the target pane's status as "blocked" before the
// agent ever came up — the process is alive but sitting on a dialog, e.g.
// Codex raising its trust_directory prompt in an untrusted directory. Message
// is herdr's own description.
type AgentNotReadyError struct {
	Message string
	wrapped error
}

func (e *AgentNotReadyError) Error() string { return e.wrapped.Error() }
func (e *AgentNotReadyError) Unwrap() error { return e.wrapped }

// candidateCwdPattern extracts the first candidate's cwd out of an
// agent_name_taken error's Message, e.g. "...candidates: terminal_id=...
// pane_id=... cwd=/path/to/worktree status=Working".
var candidateCwdPattern = regexp.MustCompile(`\bcwd=(\S+)`)

// run shells out to herdr with args and returns its combined output,
// wrapping a non-zero exit with the command line and output for context. If
// the failure is herdr's JSON error envelope with code "agent_name_taken",
// "agent_blocked", "agent_name_lost", or "agent_not_ready", the returned
// error is an *AgentNameTakenError, *AgentBlockedError, *AgentNameLostError,
// or *AgentNotReadyError instead.
func run(args ...string) ([]byte, error) {
	out, err := runCommand(args...)
	if err != nil {
		wrapped := fmt.Errorf("$ herdr %s\n\n%w\n\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		var resp struct {
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(out, &resp); jsonErr == nil && resp.Error != nil {
			switch resp.Error.Code {
			case "agent_name_taken":
				candidateCwd := ""
				if m := candidateCwdPattern.FindStringSubmatch(resp.Error.Message); len(m) == 2 {
					candidateCwd = m[1]
				}
				return nil, &AgentNameTakenError{Message: resp.Error.Message, CandidateCwd: candidateCwd, wrapped: wrapped}
			case "agent_blocked":
				return nil, &AgentBlockedError{Message: resp.Error.Message, wrapped: wrapped}
			case "agent_name_lost":
				return nil, &AgentNameLostError{Message: resp.Error.Message, wrapped: wrapped}
			case "agent_not_ready":
				return nil, &AgentNotReadyError{Message: resp.Error.Message, wrapped: wrapped}
			}
		}
		return nil, wrapped
	}
	return out, nil
}

// runJSON shells out to herdr with args and unmarshals the `result` field of
// its JSON response into result (a pointer), unless result is nil.
func runJSON(args []string, result any) error {
	out, err := run(args...)
	if err != nil {
		return err
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return fmt.Errorf("parsing herdr %s output: %w", strings.Join(args, " "), err)
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(resp.Result, result); err != nil {
		return fmt.Errorf("parsing herdr %s result: %w", strings.Join(args, " "), err)
	}
	return nil
}

// appendFlag appends flag and value to args, unless value is empty.
func appendFlag(args []string, flag, value string) []string {
	if value == "" {
		return args
	}
	return append(args, flag, value)
}
