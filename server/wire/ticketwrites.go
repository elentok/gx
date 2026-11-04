package wire

// Refusal reasons of the ticket writes (pause, relaunch, unmute).
const (
	ReasonReasonRequired   = "reason-required"
	ReasonAlreadyPaused    = "already-paused"
	ReasonNotMuted         = "not-muted"
	ReasonIterationRunning = "iteration-running"
	ReasonTicketDone       = "ticket-done"
	ReasonHerdrUnavailable = "herdr-unavailable"
	// ReasonAgentWorking refuses a retry while the ticket's agent is still
	// working; the web maps it to "Stop agent and retry".
	ReasonAgentWorking = "agent-working"
	// ReasonBudgetLimit refuses a relaunch or nudge past the daily soft
	// limit (the hard limit implies it), as claims are held back.
	ReasonBudgetLimit = "budget-limit"
	// ReasonUsageLimit refuses an automatic relaunch while the usage gate holds.
	ReasonUsageLimit = "usage-limit"
)
