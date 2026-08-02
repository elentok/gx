package cmd

import (
	"fmt"
	"io"

	"github.com/elentok/gx/tickets/schema"
)

// runTicketsValidate parses path via schema.ParseTicket and reports whether
// it's well-formed. A parse/validation failure is returned as an error (for
// a non-zero exit code via cobra's RunE); success prints a confirmation to w.
func runTicketsValidate(path string, w io.Writer) error {
	ticket, err := schema.ParseTicket(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s: valid ticket (id=%s, status=%s)\n", path, ticket.ID, ticket.Status)
	return nil
}
