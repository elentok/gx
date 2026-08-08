package cmd

import (
	"fmt"
	"io"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// runTicketsEpics resolves the local ticket tracker's canonical `.scratch`
// root from cwd and prints its epic directory names, one per line,
// alphabetically sorted (the order os.ReadDir already returns) with
// `.archive` and other dot-prefixed directories excluded by tickets.Load —
// bare slugs only, no path-building, so it composes with `gx tickets root`
// in shell tooling like `cd $(gx tickets root)/$(gx tickets epics | fzf)`.
func runTicketsEpics(cwd string, w io.Writer) error {
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return fmt.Errorf("not inside a git repo: %w", err)
	}

	epics, err := tickets.Load(repo.ScratchRoot())
	if err != nil {
		return err
	}

	for _, epic := range epics {
		fmt.Fprintln(w, epic.Name)
	}
	return nil
}
