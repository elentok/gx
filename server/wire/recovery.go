package wire

// Refusal reasons of approve.
const (
	ReasonNoProposal    = "no-proposal"
	ReasonProposalStale = "proposal-stale"
	ReasonRemedyFailed  = "remedy-failed"
)

// Refusal reasons of release-gate and finish.
const (
	ReasonAgentBusy      = "agent-busy"
	ReasonFinishTimedOut = "finish-timed-out"
)

// Refusal reasons of set-parent.
const (
	ReasonParentChanged = "parent-changed"
	ReasonInvalidParent = "invalid-parent"
)

// TicketInfo.Recovery values.
const (
	RecoveryPending   = "pending"
	RecoveryEscalated = "escalated"
)

// EventMergeRolledBack streams when startup recovery found a merge that never
// reached the feature branch and undid what it left half done.
const EventMergeRolledBack = "land-rolled-back"
