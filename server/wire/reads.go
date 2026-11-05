package wire

import (
	"time"

	"github.com/elentok/gx/ralphloop/runlog"
)

// ProjectInfo is one project in the /v1/projects payload.
type ProjectInfo struct {
	Name string `json:"name"`
	Repo string `json:"repo,omitempty"`
	VCS  string `json:"vcs,omitempty"`
	// Tickets counts the project's tickets by status.
	Tickets map[string]int `json:"tickets"`
}

// LockInfo is one held lock in the /v1/locks payload. Alive is false for a lock
// a crashed process left behind.
type LockInfo struct {
	Kind   string    `json:"kind"`
	Epic   string    `json:"epic"` // "project:epic"
	Pid    int       `json:"pid"`
	Since  time.Time `json:"since"`
	Ticket string    `json:"ticket,omitempty"`
	Alive  bool      `json:"alive"`
}

// History is the /v1/tickets/history payload: the ticket's events in log order.
type History struct {
	Address string         `json:"address"`
	Events  []runlog.Event `json:"events"`
}

// Explanation is the /v1/tickets/explain payload: why the scheduler would or
// wouldn't pick the ticket. Verdict is the scheduler's own decision word
// (blocked, error, stalled, done, ...) or one of the queue and slot verdicts
// in verdicts.go.
type Explanation struct {
	Address string `json:"address"`
	Verdict string `json:"verdict"`
	Reason  string `json:"reason,omitempty"`
}
