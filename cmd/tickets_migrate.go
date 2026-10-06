package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// runTicketsMigrateToStore copies the old tracker tree at oldRoot into the
// ticket store as the project for the repo at cwd (see
// tickets.MigrateIntoStore). The project is named projectFlag, else after the
// repo directory: for a bare layout, the directory holding .bare.
func runTicketsMigrateToStore(cwd, oldRoot, projectFlag string, dryRun bool, w io.Writer) error {
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return fmt.Errorf("not inside a git repo: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.TicketStore.Path == "" {
		return errors.New("ticket-store.path is not set")
	}

	name := projectFlag
	if name == "" {
		dir := repo.Root
		if filepath.Base(dir) == ".bare" {
			dir = filepath.Dir(dir)
		}
		name = filepath.Base(dir)
	}

	n, err := tickets.MigrateIntoStore(oldRoot, filepath.Join(cfg.TicketStore.Path, name), name, repo.Root, dryRun)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintf(w, "dry run: would copy %d file(s) into project %q; all tickets valid\n", n, name)
		return nil
	}
	fmt.Fprintf(w, "copied %d file(s) into project %q\n", n, name)
	return nil
}

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
