package wire

// Refusal reasons of the answer verb.
const (
	ReasonStale         = "stale"
	ReasonNotPaused     = "not-paused"
	ReasonNotAnswerable = "not-answerable"
	ReasonBadChoice     = "bad-choice"
	ReasonEmptyAnswer   = "empty-answer"
	ReasonTooLong       = "too-long"
)
