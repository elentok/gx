package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

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
// file untouched and this returns that validation error unchanged. It then
// backfills merged_at on already-landed epics (see tickets.BackfillMergedAt)
// and moves live "## Needs Answer" sections into the ticket log (see
// tickets.MigrateParkSections). Last, it rewrites the old needs-answer and
// needs-repair tickets to paused (see tickets.PlanPauseMigration); with dryRun
// it only prints that plan.
func runTicketsMigrate(scratchRoot string, dryRun bool, w io.Writer) error {
	steps, err := tickets.PlanPauseMigration(scratchRoot)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintln(w, "dry run: pause migration only, nothing written")
		for _, s := range steps {
			fmt.Fprintf(w, "%s: %s\n", s.Path, s.Note())
		}
		fmt.Fprintf(w, "%d file(s) would change\n", len(steps))
		return nil
	}
	result, err := tickets.Migrate(scratchRoot)
	if err != nil {
		return err
	}
	backfilled, err := tickets.BackfillMergedAt(scratchRoot, epicLandedUnder(scratchRoot))
	changes := append(result.Changes, backfilled...)
	if err == nil {
		var timed []tickets.MigrationChange
		timed, err = tickets.BackfillEpicTimes(scratchRoot)
		changes = append(changes, timed...)
	}
	if err == nil {
		var moved []tickets.MigrationChange
		moved, err = tickets.MigrateQuestionSections(scratchRoot, time.Now())
		changes = append(changes, moved...)
	}
	if err == nil {
		var paused []tickets.MigrationChange
		paused, err = tickets.ApplyPauseMigration(steps)
		changes = mergeChanges(changes, paused)
	}

	if len(changes) == 0 && err == nil {
		fmt.Fprintln(w, "no changes")
		return nil
	}

	for _, change := range changes {
		fmt.Fprintf(w, "%s: %s\n", change.Path, strings.Join(change.Notes, ", "))
	}
	fmt.Fprintf(w, "%d file(s) changed\n", len(changes))
	return err
}

// mergeChanges adds extra to changes, folding a file that already has an entry
// into it so the file count stays one per file.
func mergeChanges(changes, extra []tickets.MigrationChange) []tickets.MigrationChange {
	for _, e := range extra {
		i := slices.IndexFunc(changes, func(c tickets.MigrationChange) bool { return c.Path == e.Path })
		if i < 0 {
			changes = append(changes, e)
			continue
		}
		changes[i].Notes = append(changes[i].Notes, e.Notes...)
	}
	return changes
}

// epicLandedUnder reports whether an epic of the tracker root's repo has
// landed: its branch is gone, or is an ancestor of its target (its base,
// else trunk). The repo is resolved on first use, so a root with nothing to
// backfill never needs one.
func epicLandedUnder(root string) func(tickets.Epic) (bool, error) {
	var repoRoot, trunk string
	return func(e tickets.Epic) (bool, error) {
		if repoRoot == "" {
			var err error
			if repoRoot, trunk, err = trackerRepo(root); err != nil {
				return false, fmt.Errorf("backfilling merged_at: %w", err)
			}
		}
		if !git.RefExists(repoRoot, "refs/heads/"+e.Name) {
			return true, nil
		}
		target := e.Base
		if target == "" {
			target = trunk
		}
		return git.IsAncestor(repoRoot, e.Name, target)
	}
}

// trackerRepo is the repo and trunk of a tracker root: project.json's repo
// for a ticket-store project, else the repo holding an old .scratch tree.
func trackerRepo(root string) (repoRoot, trunk string, err error) {
	if pf, err := config.ReadProjectFile(root); err == nil && pf.Repo != nil && *pf.Repo != "" {
		trunk, err := tickets.Trunk(root, *pf.Repo)
		return *pf.Repo, trunk, err
	}
	repo, err := tickets.OpenRepo(root)
	if err != nil {
		return "", "", err
	}
	return repo.Root, repo.Trunk, nil
}
