package cmd

import (
	"io"
	"path/filepath"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// verifyContextWindow is larger than the code-review stub's: verify drives a
// running gx and reads the spec.
const verifyContextWindow = 70000

// runTicketsEnsureVerify is runTicketsEnsureCodeReview for the epic's `type:
// verify` ticket: a no-op when one exists, else it stamps out an open stub.
func runTicketsEnsureVerify(epicPath string, w io.Writer) error {
	return ensureTypedTicket(epicPath, w, typedTicketSpec{
		ticketType:    schema.TypeVerify,
		name:          "verify",
		contextWindow: verifyContextWindow,
		bodyArgs:      []any{verifySpecLine(epicPath)},
		bodyFormat:    "\n# %s — Verify: %s\n\n## What to verify\n\n%s\n## Test seams\n\nnone — verify ticket, opens child tickets (`parent:` pointing back here) if it finds anything.\n\n## Acceptance criteria\n\n- [ ] The finished epic checked in a running gx\n- [ ] Any findings opened as child tickets\n",
	})
}

// verifySpecLine names the epic's spec, or is empty when there is none.
func verifySpecLine(epicPath string) string {
	if spec := tickets.EpicSpecPath(filepath.Clean(epicPath)); spec != "" {
		return "Spec: `" + spec + "`\n\n"
	}
	return ""
}
