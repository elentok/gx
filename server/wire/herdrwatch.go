package wire

const (
	// EventHerdrUnavailable and EventHerdrAvailable are emitted on transition
	// only, never per failed retry.
	EventHerdrUnavailable = "herdr-unavailable"
	EventHerdrAvailable   = "herdr-available"
)
