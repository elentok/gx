// Package subagent holds the wire types of a subagent's page. They sit in a
// package of their own because server/wire is at its size limit.
package subagent

// Statuses of a SessionSubagent.
const (
	SubagentStatusRunning = "running"
	SubagentStatusDone    = "done"
	SubagentStatusStopped = "stopped"
	// SubagentStatusNotAvailable is a call whose file is gone.
	SubagentStatusNotAvailable = "not_available"
)

// Reasons a subagent's page has no content, on SessionSubagent.Unavailable.
const (
	SubagentUnavailableStarting   = "starting"
	SubagentUnavailableGone       = "gone"
	SubagentUnavailableCompressed = "compressed"
)

// SessionSubagentLink is one call above a subagent: its call id and title.
type SessionSubagentLink struct {
	CallID string `json:"call_id"`
	Title  string `json:"title,omitempty"`
}

// SessionSubagent is the header of one subagent's page. A field with no value
// is absent. EndedAt is absent while it runs and when its end time is unknown.
type SessionSubagent struct {
	CallID        string `json:"call_id"`
	ParentCallID  string `json:"parent_call_id,omitempty"`
	RootSessionID string `json:"root_session_id"`
	// Chain lists the calls above this one, outermost first.
	Chain       []SessionSubagentLink `json:"chain,omitempty"`
	Title       string                `json:"title,omitempty"`
	Type        string                `json:"type,omitempty"`
	Status      string                `json:"status"`
	Model       string                `json:"model,omitempty"`
	StartedAt   string                `json:"started_at,omitempty"`
	EndedAt     string                `json:"ended_at,omitempty"`
	Tokens      int                   `json:"tokens,omitempty"`
	ToolCalls   int                   `json:"tool_calls,omitempty"`
	Unavailable string                `json:"unavailable,omitempty"`
	Prompt      string                `json:"prompt,omitempty"`
	// Result is the handback message; for a Stopped subagent that left a
	// finished marker it is the last assistant message, which the page mutes.
	Result string `json:"result,omitempty"`
}
