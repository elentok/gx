package merge

// TicketTrailerKey is the commit-message trailer a merging stamps onto every
// merged commit and verify's last-resort fallback searches for.
const TicketTrailerKey = "Ralph-Loop-Ticket"

// The trailers stamped alongside TicketTrailerKey whenever the merging
// session's own metrics are available: the same actual_context_window,
// elapsed_time and cost written to the ticket's frontmatter, surfaced on the
// merged commit too.
const (
	TokensTrailerKey  = "Ralph-Loop-Tokens"
	ElapsedTrailerKey = "Ralph-Loop-Elapsed"
	CostTrailerKey    = "Ralph-Loop-Cost"
)

// TicketTrailerValue builds TicketTrailerKey's value, scoped to epicName: a
// bare ticket identifier repeats across every epic (each restarts numbering
// from 01), and TrailerCommitExists searches the whole repo history, not just
// commits under this epic. An unscoped value can match a same-numbered
// ticket from a completely unrelated epic merged long ago, misreporting a
// genuinely unmerged ticket as already present.
func TicketTrailerValue(epicName, identifier string) string {
	return epicName + "/" + identifier
}
