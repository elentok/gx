package ticketsflow

import (
	"io"

	"github.com/elentok/gx/tickets/schema"
)

// runTicketsEnsureCodeReview checks whether epicPath already has a `type:
// code-review` ticket among its published issues. If one exists, it's a
// no-op. Otherwise it stamps out a stub ticket (next sequential ID, status
// open, type code-review, "what to review" naming the epic's spec and commit
// prefix) and
// validates it the same way `gx tickets validate` does before reporting
// success. This is the single source of truth for "does this epic have a
// code-review ticket, add one if not" per
// .scratch/gx-cleanup/issues/03-ensure-code-review.md — gx-to-tickets and the
// gx-cleanup skill call this instead of each carrying their own copy of the
// check-and-create logic.
func runTicketsEnsureCodeReview(epicPath string, w io.Writer) error {
	return ensureTypedTicket(epicPath, w, typedTicketSpec{
		ticketType: schema.TypeCodeReview,
		name:       "code-review",

		contextWindow: 30000,
		bodyArgs:      []any{epicSpecLine(epicPath)},
		bodyFormat:    "\n# %s — Code review: %s\n\n## What to review\n\n%sThe epic's commits: those whose subject starts with `%[2]s/`.\n\n## Test seams\n\nnone — review ticket, opens fix tickets as children (`parent:` pointing back here) if it finds anything.\n\n## Acceptance criteria\n\n- [ ] Full epic reviewed for correctness and cross-ticket consistency\n- [ ] Any findings opened as child fix tickets\n",
	})
}
