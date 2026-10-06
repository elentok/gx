package cmd

import (
	"fmt"
	"io"

	"github.com/elentok/gx/tickets"
)

// runTicketsEpics resolves the cwd repo's ticket root (see ticketRoot) and
// prints its epic directory names, one per line,
// alphabetically sorted (the order os.ReadDir already returns) with
// `.archive` and other dot-prefixed directories excluded by tickets.Load —
// bare slugs only, no path-building, so it composes with `gx tickets root`
// in shell tooling like `cd $(gx tickets root)/$(gx tickets epics | fzf)`.
// When mapsOnly is set, only epics with a wayfinder map.md (Epic.IsMap) are
// printed.
func runTicketsEpics(cwd string, w io.Writer, mapsOnly bool) error {
	root, err := ticketRoot(cwd)
	if err != nil {
		return err
	}

	epics, err := tickets.Load(root)
	if err != nil {
		return err
	}

	for _, epic := range epics {
		if mapsOnly && !epic.IsMap {
			continue
		}
		fmt.Fprintln(w, epic.Name)
	}
	return nil
}
