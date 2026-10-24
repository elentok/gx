package cmd

import (
	"io"

	"github.com/elentok/gx/tickets/schema"
)

// runTicketsEnsureVerify is runTicketsEnsureCodeReview for the epic's `type:
// verify` ticket: a no-op when one exists, else it stamps out an open stub.
func runTicketsEnsureVerify(epicPath string, w io.Writer) error {
	return ensureTypedTicket(epicPath, w, typedTicketSpec{
		ticketType: schema.TypeVerify,
		name:       "verify",
		bodyFormat: "\n# %s — Verify: %s\n\n## What to verify\n\n\n## Test seams\n\nnone — verify ticket, opens child tickets (`parent:` pointing back here) if it finds anything.\n\n## Acceptance criteria\n\n- [ ] The finished epic checked in a running gx\n- [ ] Any findings opened as child tickets\n",
	})
}
