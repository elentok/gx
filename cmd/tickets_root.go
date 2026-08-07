package cmd

import (
	"fmt"
	"io"

	"github.com/elentok/gx/git"
)

// runTicketsRoot resolves the local ticket tracker's canonical `.scratch`
// root from cwd and prints it to w with no trailing decoration, so it's
// safe to use as `$(gx tickets root)`.
func runTicketsRoot(cwd string, w io.Writer) error {
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return fmt.Errorf("not inside a git repo: %w", err)
	}
	fmt.Fprintln(w, repo.ScratchRoot())
	return nil
}
