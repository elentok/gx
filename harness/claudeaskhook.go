package harness

import "encoding/json"

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
