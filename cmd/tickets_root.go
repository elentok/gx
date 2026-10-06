package cmd

import (
	"fmt"
	"io"

	"github.com/elentok/gx/tickets"
)

// ticketRoot resolves cwd's repo to its project directory in the ticket store.
func ticketRoot(cwd string) (string, error) {
	return tickets.RootFor(cwd)
}

// runTicketsRoot prints the cwd repo's ticket root (its project directory in
// the ticket store) to w with no trailing decoration, so it's safe to use as
// `$(gx tickets root)`.
func runTicketsRoot(cwd string, w io.Writer) error {
	root, err := ticketRoot(cwd)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, root)
	return nil
}
