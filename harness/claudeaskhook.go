package harness

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// AskHoldDirEnv carries the holder's state dir into the child's env, so the
// hook can leave a marker where the watcher looks.
const AskHoldDirEnv = "GX_HOLDER_DIR"

const askHoldsDir = "ask-holds"

// AskHoldMarker creates the marker of a hook holding in dir, named by its pid,
// and returns the func that removes it. A dir of "" makes no marker.
func AskHoldMarker(dir string, pid int) (remove func(), err error) {
	if dir == "" {
		return func() {}, nil
	}
	holds := filepath.Join(dir, askHoldsDir)
	if err := os.MkdirAll(holds, 0o700); err != nil {
		return func() {}, err
	}
	path := filepath.Join(holds, strconv.Itoa(pid))
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return func() {}, err
	}
	return func() { _ = os.Remove(path) }, nil
}

// askHeld says whether a live hook holds a question in dir. A marker whose pid
// is gone (a hook that was killed) is stale: it is deleted and ignored.
func askHeld(dir string) bool {
	holds := filepath.Join(dir, askHoldsDir)
	entries, err := os.ReadDir(holds)
	if err != nil {
		return false
	}
	held := false
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			_ = os.Remove(filepath.Join(holds, e.Name()))
			continue
		}
		held = true
	}
	return held
}

// ClaudeAskHookCommand is the PreToolUse hook that holds Claude's
// AskUserQuestion dialog for the web. The gx session id travels in the env
// (holder.AgentSessionEnv), which every gx launch already sets.
const ClaudeAskHookCommand = "gx agent ask-question"

// ClaudeAskHookTimeoutSeconds is how long Claude waits on the hook before the
// terminal dialog takes over.
const ClaudeAskHookTimeoutSeconds = 600

// ClaudeAskHookArgs is the --settings flag that installs the AskUserQuestion
// hook. MergeClaudeSettings folds it into the launch's one --settings flag.
func ClaudeAskHookArgs() []string {
	settings := map[string]any{"hooks": map[string]any{"PreToolUse": []any{map[string]any{
		"matcher": "AskUserQuestion",
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": ClaudeAskHookCommand,
			"timeout": ClaudeAskHookTimeoutSeconds,
		}},
	}}}}
	data, _ := json.Marshal(settings)
	return []string{settingsFlag, string(data)}
}
