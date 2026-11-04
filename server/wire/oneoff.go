package wire

// OneOffRequest is the body of a submit. Only Prompt is required.
type OneOffRequest struct {
	Prompt string `json:"prompt"`
	// Project wins over Cwd; with neither matching, the ticket merges in scratch.
	Project string `json:"project,omitempty"`
	// Cwd is the caller's directory, resolved to the registered project owning it.
	Cwd   string `json:"cwd,omitempty"`
	Name  string `json:"name,omitempty"`
	Agent string `json:"agent,omitempty"`
	// Type is the ticket type: "prompt" (default), "evolve" or "diagnose". A
	// one-off never commits.
	Type string `json:"type,omitempty"`
	// BlockedBy are addresses of tickets in the same project.
	BlockedBy             []string `json:"blocked_by,omitempty"`
	ExpectedContextWindow int      `json:"expected_context_window,omitempty"`
	// Front queues it at the head instead of the tail.
	Front bool `json:"front,omitempty"`
	// Notify sends the ticket's Result to chat on success; pauses always notify.
	Notify bool `json:"notify,omitempty"`
	// File is a server-side path to a markdown payload (frontmatter + body).
	// Its frontmatter fills every field left unset here; the body is the prompt
	// when Prompt is empty.
	File string `json:"file,omitempty"`
	// Unique is the dedupe key; a --file submit defaults it to the file's
	// resolved path unless NoUnique is set. A plain prompt has none.
	Unique   string `json:"unique,omitempty"`
	NoUnique bool   `json:"no_unique,omitempty"`
}

// OneOffResult is a submit's answer; a refusal creates nothing.
type OneOffResult struct {
	Address string `json:"address,omitempty"`
	Status  string `json:"status,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Refusal reasons of a one-off submit.
const (
	ReasonEmptyPrompt   = "empty-prompt"
	ReasonUnknownProj   = "unknown-project"
	ReasonOptionRemoved = "option-removed"
	ReasonBadBlocker    = "invalid-blocker"
	ReasonBadWindow     = "invalid-context-window"
	ReasonBadFile       = "invalid-file"
	ReasonDuplicateLive = "duplicate-live"
	ReasonBadType       = "invalid-type"
)
