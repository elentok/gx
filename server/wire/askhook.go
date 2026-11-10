package wire

import "encoding/json"

// AskHookRequest is Claude's AskUserQuestion call as its PreToolUse hook sees it.
// The server holds it until it has a result.
type AskHookRequest struct {
	// Session is the gx session id the hook runs in.
	Session   string `json:"session"`
	ToolUseID string `json:"tool_use_id"`
	// Questions are the dialog's questions, in order.
	Questions []AskHookQuestion `json:"questions"`
}

// AskHookQuestion is one question of the dialog.
type AskHookQuestion struct {
	Question    string          `json:"question"`
	Header      string          `json:"header,omitempty"`
	MultiSelect bool            `json:"multi_select,omitempty"`
	Options     []AskHookOption `json:"options,omitempty"`
}

// AskHookOption is one option of a question.
type AskHookOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// What the hook does with the held call.
const (
	// AskHookAllow lets the tool run with UpdatedInput, which answers it.
	AskHookAllow = "allow"
	// AskHookDeny blocks the tool call; Reason goes back to the agent.
	AskHookDeny = "deny"
	// AskHookRelease prints nothing, so the terminal dialog shows.
	AskHookRelease = "release"
)

// AskHookResult is the server's decision for a held question.
type AskHookResult struct {
	Decision     string          `json:"decision"`
	Reason       string          `json:"reason,omitempty"`
	UpdatedInput json.RawMessage `json:"updated_input,omitempty"`
}
