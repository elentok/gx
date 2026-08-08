package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// runTicketsEnsureCodeReview checks whether epicPath already has a `type:
// code-review` ticket among its published issues. If one exists, it's a
// no-op. Otherwise it stamps out a stub ticket (next sequential ID, status
// ready-for-agent, type code-review, empty "what to review" body) and
// validates it the same way `gx tickets validate` does before reporting
// success. This is the single source of truth for "does this epic have a
// code-review ticket, add one if not" per
// .scratch/gx-cleanup/issues/03-ensure-code-review.md — gx-to-tickets and the
// gx-cleanup skill call this instead of each carrying their own copy of the
// check-and-create logic.
func runTicketsEnsureCodeReview(epicPath string, w io.Writer) error {
	epicPath = filepath.Clean(epicPath)
	scratchDir := filepath.Dir(epicPath)
	epicName := filepath.Base(epicPath)

	epics, err := tickets.Load(scratchDir)
	if err != nil {
		return fmt.Errorf("loading epics under %s: %w", scratchDir, err)
	}

	var epic *tickets.Epic
	for i := range epics {
		if epics[i].Name == epicName {
			epic = &epics[i]
			break
		}
	}
	if epic == nil {
		return fmt.Errorf("epic not found: %s", epicPath)
	}

	for _, t := range epic.Tickets {
		if t.Type == string(schema.TypeCodeReview) {
			fmt.Fprintf(w, "%s: already has a code-review ticket (%s)\n", epicPath, t.DisplayNumber())
			return nil
		}
	}

	id := nextTicketID(epic.Tickets)
	stubPath := filepath.Join(epicPath, "issues", fmt.Sprintf("%s-code-review.md", id))

	stub := schema.Ticket{
		ID:                    schema.TicketID(id),
		Status:                schema.StatusReadyForAgent,
		Type:                  schema.TypeCodeReview,
		ExpectedContextWindow: 30000,
	}
	body := fmt.Sprintf(
		"\n# %s — Code review: %s\n\n## What to review\n\n\n## Test seams\n\nnone — review ticket, opens fix tickets as `children` if it finds anything.\n\n## Acceptance criteria\n\n- [ ] Full epic reviewed for correctness and cross-ticket consistency\n- [ ] Any findings opened as child fix tickets\n",
		id, epicName,
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

	fmt.Fprintf(w, "%s: created code-review stub\n", stubPath)
	return nil
}

// nextTicketID returns the next sequential zero-padded ticket ID after the
// highest Number among existing, following the epic's existing numbering
// convention (see tickets/loader.go's ticketFilenameRe). A ticketless epic
// starts at "01".
func nextTicketID(existing []tickets.Ticket) string {
	max := 0
	for _, t := range existing {
		if t.Number > max {
			max = t.Number
		}
	}
	return fmt.Sprintf("%02d", max+1)
}
