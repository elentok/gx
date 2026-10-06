package cmd

import (
	"fmt"
	"io"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// ticketRoot resolves cwd's repo to its project directory in the ticket store.
func ticketRoot(cwd string) (string, error) {
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return "", fmt.Errorf("not inside a git repo: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return tickets.ProjectDir(cfg.TicketStore.Path, repo.Root)
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
