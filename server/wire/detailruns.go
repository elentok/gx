package wire

import (
	"time"
)

// Outcomes of a run line.
const (
	RunMerged          = "merged"
	RunMergedNoCommits = "merged-no-commits"
	RunPaused          = "paused"
	RunStartedOver     = "started-over"
	RunRunning         = "running"
	RunAgentLost       = "agent-lost"
	// RunAgentGoneAfterRestart is a run a restarted server found dead with no
	// work to keep, and put back in the queue.
	RunAgentGoneAfterRestart = "agent-gone-after-restart"
	RunRelaunched            = "relaunched"
	RunCancelled             = "cancelled"
)

// How a run began. The run log keeps cold-start and resumed; the page labels
// them "new session" and "same session".
const (
	StartNewSession  = "new-session"
	StartSameSession = "same-session"
)

// DetailRun is one run line of the Runs section.
type DetailRun struct {
	Start time.Time `json:"start"`
	// Duration is in seconds; a running or lost run has none.
	Duration int    `json:"duration,omitempty"`
	Outcome  string `json:"outcome"`
	// PauseLabel is the reason label of a run that ended in a pause.
	PauseLabel string `json:"pause_label,omitempty"`
	StartKind  string `json:"start_kind"`
}
