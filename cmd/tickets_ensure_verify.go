package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// runTicketsEnsureVerify is runTicketsEnsureCodeReview for the epic's `type:
// verify` ticket: a no-op when one exists, else it stamps out an open stub.
func runTicketsEnsureVerify(epicPath string, w io.Writer) error {
	epicPath = filepath.Clean(epicPath)

	epic, unlock, err := tickets.LoadLockedEpic(epicPath)
	if err != nil {
		return err
	}
	defer unlock()

	for _, t := range epic.Tickets {
		if t.Type == string(schema.TypeVerify) {
			fmt.Fprintf(w, "%s: already has a verify ticket (%s)\n", epicPath, t.DisplayNumber())
			return nil
		}
	}

	id, err := tickets.NextTicketID(*epic, "")
	if err != nil {
		return err
	}
	stubPath := filepath.Join(epicPath, "issues", fmt.Sprintf("%s-verify.md", id))

	stub := schema.Ticket{
		ID:                    schema.TicketID(id),
		Status:                schema.StatusOpen,
		Type:                  schema.TypeVerify,
		ExpectedContextWindow: 30000,
	}
	address, ok := tickets.AddressOfPath(stubPath, id)
	if !ok {
		return fmt.Errorf("no address for new ticket %s", stubPath)
	}
	stub.StampCreated(time.Now().UTC(), address.String())
	body := fmt.Sprintf(
		"\n# %s — Verify: %s\n\n## What to verify\n\n\n## Test seams\n\nnone — verify ticket, opens child tickets (`parent:` pointing back here) if it finds anything.\n\n## Acceptance criteria\n\n- [ ] The finished epic checked in a running gx\n- [ ] Any findings opened as child tickets\n",
		id, epic.Name,
	)

	out, err := schema.MarshalTicket(stub, body)
	if err != nil {
		return fmt.Errorf("marshaling stub ticket: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(stubPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(stubPath, out, 0644); err != nil {
		return fmt.Errorf("writing stub ticket %s: %w", stubPath, err)
	}

	if _, err := schema.ParseTicket(stubPath); err != nil {
		return fmt.Errorf("stub ticket %s failed validation: %w", stubPath, err)
	}

	fmt.Fprintf(w, "%s: created verify stub\n", stubPath)
	return nil
}
