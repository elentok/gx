// Package herdr wraps the herdr CLI's workspace/worktree/tab/agent socket-API
// commands as plain functions returning (result, error). It knows nothing
// about bubbletea, so both the worktrees UI (via tea.Cmd wrappers) and
// non-TUI callers like ralph-loop can shell out to herdr directly.
package herdr

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// runCommand is a seam so tests can fake `herdr` invocations without
// spawning the real executable.
var runCommand = func(args ...string) ([]byte, error) {
	return exec.Command("herdr", args...).CombinedOutput()
}

// run shells out to herdr with args and returns its combined output,
// wrapping a non-zero exit with the command line and output for context.
func run(args ...string) ([]byte, error) {
	out, err := runCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("$ herdr %s\n\n%w\n\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
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
