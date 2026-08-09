package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/elentok/gx/tickets"
)

// runTicketsMigrate rewrites every ticket under scratchRoot into the
// post-refactor frontmatter shape (see tickets.Migrate) and reports what
// changed, per file, to w. tickets.Migrate validates the whole computed
// result — every ticket's frontmatter plus every epic's parent graph —
// before writing anything, so a result that would be invalid leaves every
// file untouched and this returns that validation error unchanged.
func runTicketsMigrate(scratchRoot string, w io.Writer) error {
	result, err := tickets.Migrate(scratchRoot)
	if err != nil {
		return err
	}

	if len(result.Changes) == 0 {
		fmt.Fprintln(w, "no changes")
		return nil
	}

	for _, change := range result.Changes {
		fmt.Fprintf(w, "%s: %s\n", change.Path, strings.Join(change.Notes, ", "))
	}
	fmt.Fprintf(w, "%d file(s) changed\n", len(result.Changes))
	return nil
}
