package wire

// IterationInfo is one live iteration in the /v1/iterations payload. The
// transcript path is only located here: parsing it belongs to the transcript
// module.
type IterationInfo struct {
	Address string `json:"address"`
	Agent   string `json:"agent,omitempty"`
	Pane    string `json:"pane"`
	Tab     string `json:"tab,omitempty"`
	// Runner names what hosts the iteration; QueueAgent is the PTY run's row id
	// in the sessions listing, empty for every other runner.
	Runner     string `json:"runner,omitempty"`
	QueueAgent string `json:"queueAgent,omitempty"`
	Worktree   string `json:"worktree"`
	Branch     string `json:"branch"`
	Base       string `json:"base,omitempty"`
	Session    string `json:"session,omitempty"`
	Transcript string `json:"transcript,omitempty"`
}

// QueueEntry is one ticket in the /v1/queue payload with the scheduler's own
// verdict on it.
type QueueEntry struct {
	Address  string `json:"address"`
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}
