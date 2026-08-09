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
	if err := checkParentGraph(path); err != nil {
		return err
	}
	fmt.Fprintf(w, "%s: valid ticket (id=%s, status=%s)\n", path, ticket.ID, ticket.Status)
	return nil
}

// checkParentGraph reports path's parent edge as invalid when the epic around
// it says so — dangling, or closing a cycle. Neither is visible to
// schema.Validate, which sees one file with no epic around it; the loader
// records the verdict on the ticket it drops the edge from (Ticket.GraphErr).
// A ticket with no loadable epic (an ad-hoc file, or an epic that can't be
// read) is validated on its own frontmatter alone.
func checkParentGraph(path string) error {
	_, target, unlock, err := lockEpicForTicket(path)
	if err != nil {
		return nil
	}
	if unlock != nil {
		defer unlock()
	}
	if target == nil || target.GraphErr == "" {
		return nil
	}
	return fmt.Errorf("%s: %s", path, target.GraphErr)
}
