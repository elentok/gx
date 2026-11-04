package wire

// Refusal reasons of the agent verbs. iteration-not-live is shared with nudge.
const (
	ReasonAgentNotReady = "agent-not-ready"
	ReasonNotBlocked    = "not-blocked"
	ReasonBadDecision   = "invalid-decision"
	// With a request ID: the open request differs, needs a real answer, or the agent is gone.
	ReasonRequestChanged = "request-changed"
	ReasonNotApprovable  = "not-approvable"
	// ReasonRequestIDRequired is the web's refusal of an answer that names no request.
	ReasonRequestIDRequired = "request-id-required"
	ReasonAgentGone         = "agent-gone"
	// ReasonAgentWaiting is the refusal of a plain resume on a live waiting
	// self-reported pause. It is public (ADR 0028) and named in the tracker text.
	ReasonAgentWaiting = "agent-waiting"
	// ReasonNotDelivered: a deny closed the dialog but its reason never reached the agent.
	ReasonNotDelivered = "reason-not-delivered"
)
