package wire

import (
	"time"
)

// EpicState is EpicSummary.State.
type EpicState string

const (
	EpicStateAwaitingMerge EpicState = "awaiting-merge"
	EpicStateFinished      EpicState = "finished"
	EpicStateRunning       EpicState = "running"
	// EpicStateMerging is an epic whose only unfinished tickets are merge tickets
	// that are open or claimed.
	EpicStateMerging EpicState = "merging"
	EpicStateWaiting EpicState = "waiting"
	// EpicStatePaused is an epic with nothing runnable: a paused ticket, a
	// dependency deadlock, or a blocked merge. EpicSummary.Reason says which.
	EpicStatePaused EpicState = "paused"
)

// The reasons a Paused epic gives for itself, when no ticket is paused.
const (
	EpicReasonDeadlock     = "Dependency deadlock"
	EpicReasonMergeBlocked = "Merge blocked"
)

// SummaryTicket is one ticket in an EpicSummary: an active one carries
// ClaimedAt, a Paused one carries Recovery and its unclipped Pause reason.
type SummaryTicket struct {
	Address   string    `json:"address"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	ClaimedAt time.Time `json:"claimed_at,omitzero"`
	Recovery  string    `json:"recovery,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	// ReasonLabel is the plain-words label of the ticket's pause reason.
	ReasonLabel string `json:"reason_label,omitempty"`
	// Summary is the open question's summary line, or the pause reason without one.
	Summary string `json:"summary,omitempty"`
	// Answerable is set when a person's reply can go to the open question.
	Answerable bool `json:"answerable"`
	// Answered is set on an open ticket whose latest question has an answer.
	Answered bool `json:"answered"`
	// NeedsApproval is set on an active ticket whose agent waits on a permission prompt.
	NeedsApproval bool `json:"needs_approval"`
}

// EpicSummary is one epic holding a queue item, as the web page's cards show it.
type EpicSummary struct {
	Address string    `json:"address"`
	Name    string    `json:"name"`
	Project string    `json:"project"`
	State   EpicState `json:"state"`
	Done    int       `json:"done"`
	Total   int       `json:"total"`
	// Active is the running tickets, listed only while the epic is Running.
	Active []SummaryTicket `json:"active"`
	Paused []SummaryTicket `json:"parked"`
	// HasRepair is set when a paused ticket is broken (stuck or error), so
	// both clients show a fault in the alarm color instead of a question.
	HasRepair bool `json:"has_repair"`
	// Reason is why a Paused epic is paused: the reason of its first paused
	// ticket, else "Dependency deadlock" or "Merge blocked". It is empty for a
	// Paused epic that holds only drafts.
	Reason string `json:"reason,omitempty"`
	// More is the number of paused tickets beyond the first, whose reason Reason shows.
	More int `json:"more,omitempty"`
	// Detail is the cycle that deadlocks the epic, or why its merge is blocked.
	Detail string `json:"detail,omitempty"`
	// MergeBlocked is set on a Paused epic whose last merge failed, so clients
	// keep offering the merge action.
	MergeBlocked bool `json:"merge_blocked,omitempty"`
	// MergeBlockKind is the pause reason of a blocked merge.
	MergeBlockKind string `json:"merge_block_kind,omitempty"`
	// Actions are the verbs of a blocked merge's pause, from the pause-to-verb table.
	Actions []string `json:"actions,omitempty"`
}
