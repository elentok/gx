package tickets

import (
	"fmt"
	"io"
	"strings"

	"github.com/elentok/gx/cmd/ticketref"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// sectionOptions are the flags of `gx tickets section`. The command keeps
// Append and Clear apart (MarkFlagsMutuallyExclusive).
type sectionOptions struct {
	Clear  bool
	Append bool
	JSON   bool
}

// runTicketsSection replaces (or appends) the "## heading" section of path's
// ticket body with content, under the per-ticket lock, and prints the ticket's
// address. Blank content is refused unless Clear is set: an empty stdin (a
// probe piped through head, a failed upstream command) must not wipe a section.
func runTicketsSection(d Deps, path, heading, content string, opts sectionOptions, w io.Writer) error {
	jsonOut := opts.JSON
	if strings.TrimSpace(content) == "" && !opts.Clear {
		return fmt.Errorf("refusing to blank section %q: content is empty (pass --clear to blank it on purpose)", heading)
	}
	setSection, setEpicSection := schema.SetSection, tickets.SetEpicSection
	if opts.Append {
		setSection, setEpicSection = schema.AppendSection, tickets.AppendEpicSection
	}
	if ticketref.IsEpicTicketMD(path) {
		// An epic's frontmatter is not a ticket's, so it is edited in place
		// rather than parsed as a Ticket.
		if err := setEpicSection(path, heading, content); err != nil {
			return err
		}
		return PrintTicketRef(path, jsonOut, w)
	}
	var verr error
	err := schema.UpdateTicketWithBody(path, func(_ *schema.Ticket, body *string) {
		if heading == schema.VerifiedHeading {
			if verr = schema.ValidateVerifiedKeys(*body, content); verr != nil {
				return
			}
		}
		*body = setSection(*body, heading, content)
	})
	if err == nil {
		err = verr
	}
	if err != nil {
		return err
	}
	PingServer(d.Env, path)
	return PrintTicketRef(path, jsonOut, w)
}
