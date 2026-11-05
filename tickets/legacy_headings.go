package tickets

// The headings an old pause appended to a paused ticket's body. Read shim only:
// nothing writes them any more. They live here, not in ticketlog, because the
// migrations in this package read them and ticketlog imports tickets.
const (
	LegacyAnswerHeading = "## Needs Answer"
	LegacyRepairHeading = "## Needs Repair"
)
