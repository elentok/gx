package cmd

import (
	"io"

	"github.com/elentok/gx/tickets/schema"
)

// runTicketsSection replaces (or appends) the "## heading" section of path's
// ticket body with content, under the per-ticket lock, and prints the ticket's
// address.
func runTicketsSection(path, heading, content string, jsonOut bool, w io.Writer) error {
	err := schema.UpdateTicketWithBody(path, func(_ *schema.Ticket, body *string) {
		*body = schema.SetSection(*body, heading, content)
	})
	if err != nil {
		return err
	}
	pingServer(path)
	return printTicketRef(path, jsonOut, w)
}
