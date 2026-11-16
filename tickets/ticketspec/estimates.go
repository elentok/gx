package ticketspec

import "github.com/elentok/gx/tickets"

// SplitLimitTokens is the most work one ticket should hold. It is an
// experiment: retune it after 2 or 3 epics.
const SplitLimitTokens = 60_000

// Per-item estimates for a verify part. A part's expected tokens are
// VerifyPartBaseTokens plus its items; verify-plan packs parts under
// SplitLimitTokens. The ensure-verify stub window is a separate value.
const (
	WebFlowTokens        = 6_000
	CLICheckTokens       = 3_000
	GoTestRunTokens      = 4_000
	SkillQuoteTokens     = 2_000
	MockStateTokens      = 14_000 // includes seeding, fixture start and web-build for the state
	VerifyPartBaseTokens = 15_000 // fixture start and the ticket read

	// The collect ticket reads every part's report: a base plus a cost per part.
	VerifyCollectBaseTokens = 20_000
	VerifyCollectPartTokens = 3_500
)

func collectTokens(parts int) int {
	return VerifyCollectBaseTokens + parts*VerifyCollectPartTokens
}

// FallbackPrefixTokens stands in for a ticket's prefix when none was stamped.
const FallbackPrefixTokens = 48_000

// Work returns the ticket's own work: its actual context window minus the
// prefix every session starts with. A ticket without a stamped prefix uses
// FallbackPrefixTokens, and approximate reports that. Work is never negative.
func Work(t tickets.Ticket) (work int, approximate bool) {
	prefix := t.PrefixTokens
	if prefix <= 0 {
		prefix = FallbackPrefixTokens
		approximate = true
	}
	return max(t.ActualContextWindow-prefix, 0), approximate
}
