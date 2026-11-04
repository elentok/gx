package wire

import (
	"time"

	"github.com/elentok/gx/tickets"
)

// QueueItem is one queued ticket and the agent it will run under.
type QueueItem struct {
	Address string `json:"address"`
	Agent   string `json:"agent"`
	// EnqueuedAt is set by the server on enqueue, never taken from a client. Items
	// from a queue file written before the field existed have none.
	EnqueuedAt time.Time `json:"enqueued_at,omitzero"`
}

// QueueResult is what every queue write returns: the queue after the write, or
// a refusal (the queue is then untouched).
type QueueResult struct {
	Queue []QueueItem `json:"queue,omitempty"`
	// Mode is set by pause, resume and flush: running or paused.
	Mode QueueMode `json:"mode,omitempty"`
	// ExtraUsage is the warning an enqueue carries while the account
	// auto-purchases extra usage.
	ExtraUsage bool `json:"extra_usage,omitempty"`
	// Merged, MergeTicket and Paused are an epic merge's outcome: a direct
	// merge, the address of the merge ticket it forked, or a pause of the root
	// with PauseKind and Message.
	Merged      bool   `json:"merged,omitempty"`
	MergeTicket string `json:"merge_ticket,omitempty"`
	// TrunkConflict is the address of the trunk-conflict ticket "Stash, merge and
	// restore" added.
	TrunkConflict string `json:"trunk_conflict,omitempty"`
	Paused        bool   `json:"parked,omitempty"`
	PauseKind     string `json:"park_kind,omitempty"`
	// Outcome is an answer's result: delivered or adopted. A stale refusal
	// carries the current Question and its State token.
	Outcome  string            `json:"outcome,omitempty"`
	Question *tickets.LogEvent `json:"question,omitempty"`
	State    string            `json:"state,omitempty"`
	Refused  bool              `json:"refused,omitempty"`
	Reason   string            `json:"reason,omitempty"`
	Message  string            `json:"message,omitempty"`
}

// QueueRequest is the body of the queue writes; each uses the fields it needs.
type QueueRequest struct {
	Address string `json:"address"`
	Agent   string `json:"agent,omitempty"`
	// Position is the 1-based slot a move puts the ticket in.
	Position int `json:"position,omitempty"`
	// Project and Items are what a replace swaps in.
	Project string      `json:"project,omitempty"`
	Items   []QueueItem `json:"items,omitempty"`
	// PauseReason is the one-line reason a pause writes into the ticket.
	PauseReason string `json:"park_reason,omitempty"`
	// Stop lets a cancel stop the live pane of a claimed ticket instead of refusing.
	Stop bool `json:"stop,omitempty"`
	// Front makes an add merge at the head of the queue instead of the tail.
	Front bool `json:"front,omitempty"`
	// Text is what a nudge types into the live pane.
	Text string `json:"text,omitempty"`
	// Verb is the action a recommend stores; Text carries its diagnosis.
	Verb string `json:"verb,omitempty"`
	// Decision is allow or deny, for answering a blocked permission request.
	Decision string `json:"decision,omitempty"`
	// RequestID names the permission request a decision answers; empty answers
	// whatever is open (the TUI and CLI).
	RequestID string `json:"request_id,omitempty"`
	// Choice, Note, State and Source are an answer's choice (an option id, yes or
	// no), its note, the state token of the question it answers and where it
	// came from. The server sets Source from the request's origin and ignores
	// the body. A free-text answer uses Text.
	Choice string `json:"choice,omitempty"`
	Note   string `json:"note,omitempty"`
	State  string `json:"state,omitempty"`
	Source string `json:"source,omitempty"`
}

// Refusal reasons of the queue writes. Stable: clients switch on them.
const (
	// ReasonServerNotRunning is produced by the CLI, never the server.
	ReasonServerNotRunning = "server-not-running"
	ReasonUnknownTicket    = "unknown-ticket"
	ReasonInvestigateStop  = "investigate-stop"
	ReasonInvalidAddress   = "invalid-address"
	ReasonInvalidAgent     = "invalid-agent"
	ReasonAlreadyQueued    = "already-queued"
	ReasonNotQueued        = "not-queued"
	ReasonBadPosition      = "bad-position"
	ReasonTicketLive       = "ticket-live"
	ReasonMapEpic          = "map-epic"
	ReasonNoOpenTickets    = "no-open-tickets"
	ReasonEpicDraft        = "epic-draft"
	ReasonEpicNotOpen      = "epic-not-open"
	ReasonFollowUpsEpic    = "follow-ups-epic"
	ReasonAlreadyMerged    = "already-merged"
	ReasonMergePending     = "merge-pending"
	ReasonNotComplete      = "not-complete"
	ReasonBranchMissing    = "branch-missing"
	ReasonOneOffsEpic      = "one-offs-epic"
	ReasonNotDirtyTrunk    = "not-dirty-trunk"
	ReasonNeedsRebase      = "needs-rebase"
)
