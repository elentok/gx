package wire

// SessionRoundOption is one choice of an options card.
type SessionRoundOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Consequence string `json:"consequence,omitempty"`
}

// SessionRoundCard is one question of a Grilling round. Kind is yes-no,
// options or text; Note says why a card fell back to text.
type SessionRoundCard struct {
	Label       string               `json:"label"` // the header's question id without the Q: "8", or "8b"
	Title       string               `json:"title"`
	Body        string               `json:"body"`
	Kind        string               `json:"kind"`
	Yes         string               `json:"yes,omitempty"`
	No          string               `json:"no,omitempty"`
	Options     []SessionRoundOption `json:"options,omitempty"`
	Recommended string               `json:"recommended,omitempty"`
	Note        string               `json:"note,omitempty"`
}

// SessionNext is the Actionable a gx-design run-end line raises.
type SessionNext struct {
	Action SessionActionable `json:"action"`
}

// SessionBackgroundTask is a shell, Monitor or subagent still running.
type SessionBackgroundTask struct {
	Kind      string `json:"kind"` // transcript.BackgroundTaskShell, ...Monitor or ...Agent
	Label     string `json:"label"`
	StartedAt string `json:"started_at"`
}
