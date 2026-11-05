package tickets

import (
	"fmt"
	"io"
	"strings"

	"github.com/elentok/gx/cmd/ticketref"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// runTicketsSection replaces (or appends) the "## heading" section of path's
// ticket body with content, under the per-ticket lock, and prints the ticket's
// address. Blank content is refused unless clear is set: an empty stdin (a
// probe piped through head, a failed upstream command) must not wipe a section.
func runTicketsSection(d Deps, path, heading, content string, clear, jsonOut bool, w io.Writer) error {
	if strings.TrimSpace(content) == "" && !clear {
		return fmt.Errorf("refusing to blank section %q: content is empty (pass --clear to blank it on purpose)", heading)
	}
	if ticketref.IsEpicTicketMD(path) {
		// An epic's frontmatter is not a ticket's, so it is edited in place
		// rather than parsed as a Ticket.
		if err := tickets.SetEpicSection(path, heading, content); err != nil {
			return err
		}
		return PrintTicketRef(path, jsonOut, w)
	}
	err := schema.UpdateTicketWithBody(path, func(_ *schema.Ticket, body *string) {
		*body = schema.SetSection(*body, heading, content)
	})
	if err != nil {
		return err
	}
	PingServer(d.Env, path)
	return PrintTicketRef(path, jsonOut, w)
}
