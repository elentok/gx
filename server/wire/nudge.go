package wire

// Refusal reasons of the nudge verb.
const (
	ReasonTextRequired     = "text-required"
	ReasonIterationNotLive = "iteration-not-live"
	ReasonRateLimited      = "rate-limited"
)

// EventTicketNudged is emitted after text was typed into an iteration's pane.
const EventTicketNudged = "ticket-nudged"
