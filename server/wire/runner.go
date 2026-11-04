package wire

const (
	// EventTicketClaimed streams once the ticket file says claimed.
	EventTicketClaimed = "ticket-claimed"
	// EventIterationStarted streams once the agent has taken its prompt.
	EventIterationStarted = "iteration-started"
	// EventIterationLaunchFailed streams when the launch failed and the claim
	// was rolled back.
	EventIterationLaunchFailed = "iteration-launch-failed"
	// EventTicketDone streams once the ticket's commits are merged and the file says done.
	EventTicketDone = "ticket-done"
	// EventIterationPaused streams when a finished iteration ended without merging
	// (asking or zero commits).
	EventIterationPaused = "iteration-paused"
	// EventTicketPaused streams once a person's pause is written to the ticket file.
	EventTicketPaused = "ticket-paused"
	// EventTicketCancelled streams once per ticket a cancel wrote.
	EventTicketCancelled = "ticket-cancelled"
	// EventIterationFailed streams when finishing or merging errored; the ticket stays claimed.
	EventIterationFailed = "iteration-failed"
	// EventRootCompleted streams once every ticket of a queued root is done.
	EventRootCompleted = "root-completed"
	// EventRootPaused streams when a finished root cannot fast-forward onto its target.
	EventRootPaused = "root-paused"
)

// VerdictClaimRereadMismatch is the explain verdict for a ticket whose file
// changed since the index saw it, so the last claim pass skipped it.
const VerdictClaimRereadMismatch = "claim re-read mismatch"
