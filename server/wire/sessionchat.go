package wire

import "time"

// SessionChatMessage is one visible message of a session's conversation. ID is
// the transcript line's uuid.
type SessionChatMessage struct {
	ID   string    `json:"id"`
	Role string    `json:"role"` // transcript.ChatRoleAgent or ChatRolePerson
	Kind string    `json:"kind"` // text, prompt, command or output
	Text string    `json:"text"`
	At   time.Time `json:"at"`
	// ToolCount and LastTool describe a tools entry, which folds a run of tool
	// calls and carries no Text.
	ToolCount int    `json:"tool_count,omitempty"`
	LastTool  string `json:"last_tool,omitempty"`
	// Calls is one line per folded tool call: the tool and its command or target.
	Calls []string `json:"calls,omitempty"`
}

// SessionChatResponse is the visible messages of a session's newest
// conversation. ConversationID changes after a /clear. Unsupported marks a
// session of a harness that has no chat; its Messages stay empty. Truncated
// says earlier messages were cut to fit maxChatMessages. Round holds the cards
// of a Grilling round the agent ended its last turn with and the person has not
// answered; Next is the run-end line of that turn; Background lists the shells,
// Monitors and subagents still running.
type SessionChatResponse struct {
	ConversationID string                  `json:"conversation_id"`
	Messages       []SessionChatMessage    `json:"messages"`
	Unsupported    bool                    `json:"unsupported,omitempty"`
	Truncated      bool                    `json:"truncated,omitempty"`
	Round          []SessionRoundCard      `json:"round" tstype:"SessionRoundCard[] | null"`
	Next           *SessionNext            `json:"next"`
	Background     []SessionBackgroundTask `json:"background"`
	// Failure is the session's, as on its SessionInfo.
	Failure *SessionFailure `json:"failure,omitempty"`
	// Unreadable marks a Codex conversation that has turns but no message gx
	// could read; CodexVersion is the Codex that wrote it.
	Unreadable   bool   `json:"unreadable,omitempty"`
	CodexVersion string `json:"codex_version,omitempty"`
	// Archived marks a Codex conversation whose rollout is a .jsonl.zst: the
	// messages stay empty until the archive route decodes it.
	Archived bool `json:"archived,omitempty"`
}
