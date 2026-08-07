package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/git"
)

// runTicketsEpics resolves the local ticket tracker's canonical `.scratch`
// root from cwd and prints its epic directory names, one per line,
// alphabetically sorted (the order os.ReadDir already returns) with
// `.archive` excluded — bare slugs only, no path-building, so it composes
// with `gx tickets root` in shell tooling like `cd $(gx tickets
// root)/$(gx tickets epics | fzf)`.
func runTicketsEpics(cwd string, w io.Writer) error {
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return fmt.Errorf("not inside a git repo: %w", err)
	}

	entries, err := os.ReadDir(repo.ScratchRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".archive" {
			continue
		}
		fmt.Fprintln(w, entry.Name())
	}
	return nil
}
