package tickets

// SplitLimitTokens is the most work one ticket should hold. It is an
// experiment: retune it after 2 or 3 epics.
const SplitLimitTokens = 60_000

// FallbackPrefixTokens stands in for a ticket's prefix when none was stamped.
const FallbackPrefixTokens = 48_000

// Work returns the ticket's own work: its actual context window minus the
// prefix every session starts with. A ticket without a stamped prefix uses
// FallbackPrefixTokens, and approximate reports that. Work is never negative.
func Work(t Ticket) (work int, approximate bool) {
	prefix := t.PrefixTokens
	if prefix <= 0 {
		prefix = FallbackPrefixTokens
		approximate = true
	}
	return max(t.ActualContextWindow-prefix, 0), approximate
}
