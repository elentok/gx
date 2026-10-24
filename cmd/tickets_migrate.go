package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
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
// tickets.MigrateParkSections). It renames the epic field provisional_destination
// to destination_unconfirmed (see tickets.MigrateDestinationUnconfirmed). Last, it rewrites the old needs-answer and
// needs-repair tickets to paused (see tickets.PlanPauseMigration); with dryRun
// it only prints that plan.
func runTicketsMigrate(scratchRoot string, dryRun bool, w io.Writer) error {
	steps, err := tickets.PlanPauseMigration(scratchRoot)
	if err != nil {
		return err
	}
	if dryRun {
		mapsDone, err := tickets.MigrateMapDone(scratchRoot, true)
		if err != nil {
			return err
		}
		created, err := backfillTicketCreation(scratchRoot, true)
		if err != nil {
			return err
		}
		fmt.Fprintln(w, "dry run: pause, map-done and ticket-creation migration only, nothing written")
		for _, s := range steps {
			fmt.Fprintf(w, "%s: %s\n", s.Path, s.Note())
		}
		for _, c := range append(mapsDone, created...) {
			fmt.Fprintf(w, "%s: %s\n", c.Path, strings.Join(c.Notes, ", "))
		}
		fmt.Fprintf(w, "%d file(s) would change\n", len(steps)+len(mapsDone)+len(created))
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
		var stamped []tickets.MigrationChange
		stamped, err = tickets.BackfillTicketTotals(scratchRoot, ticketSessionTotals, conflictSessionsUnder(scratchRoot))
		changes = mergeChanges(changes, stamped)
	}
	if err == nil {
		var frictions []tickets.MigrationChange
		frictions, err = tickets.BackfillFriction(scratchRoot, ticketFrictionUnder(scratchRoot))
		changes = mergeChanges(changes, frictions)
	}
	if err == nil {
		var totals []tickets.MigrationChange
		totals, err = tickets.BackfillEpicTotals(scratchRoot, epicTrailerLinesUnder(scratchRoot))
		changes = mergeChanges(changes, totals)
	}
	if err == nil {
		var renamed []tickets.MigrationChange
		renamed, err = tickets.MigrateDestinationUnconfirmed(scratchRoot)
		changes = mergeChanges(changes, renamed)
	}
	if err == nil {
		var steps []tickets.MigrationChange
		steps, err = tickets.MigrateDesignStep(scratchRoot, designProbeUnder(scratchRoot))
		changes = mergeChanges(changes, steps)
	}
	if err == nil {
		var done []tickets.MigrationChange
		done, err = tickets.MigrateMapDone(scratchRoot, false)
		changes = mergeChanges(changes, done)
	}
	if err == nil {
		var created []tickets.MigrationChange
		created, err = backfillTicketCreation(scratchRoot, false)
		changes = mergeChanges(changes, created)
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

// designProbeUnder is the trunk checkout and repo of the project at root, for
// the design step guess. A root that is not a store project has neither.
func designProbeUnder(root string) tickets.DesignProbe {
	probe := tickets.DesignProbe{BranchExists: tickets.BranchExists}
	if _, err := config.ReadProjectFile(root); err != nil {
		return probe
	}
	store, project := filepath.Dir(root), tickets.ProjectName(root)
	probe.TrunkDir = tickets.TrunkCheckout(store, project)
	probe.Repo = tickets.ProjectRepoRoot(store, project)
	return probe
}

// backfillTicketCreation stamps created_at and the original address on the
// tickets of the ticket-store project at root from the store's git history
// (see tickets.BackfillTicketCreation). A root that is not a project of a
// git-tracked store has no history to read and is skipped.
func backfillTicketCreation(root string, dryRun bool) ([]tickets.MigrationChange, error) {
	storeRoot, epics, err := storeProjectEpics(root)
	if err != nil || epics == nil {
		return nil, err
	}
	created, err := tickets.BackfillTicketCreation(epics, storeRoot, func(rel string) (tickets.TicketOrigin, bool, error) {
		o, ok, err := git.FirstCommitOfFile(storeRoot, rel)
		return tickets.TicketOrigin{Path: o.Path, At: o.At, Root: o.Root}, ok, err
	}, dryRun)
	if err != nil {
		return created, err
	}
	// Reload: the creation step may have rewritten tickets this one reads.
	_, epics, err = storeProjectEpics(root)
	if err != nil {
		return created, err
	}
	cancelled, err := tickets.BackfillCancelledAt(epics, storeRoot, func(rel string) (tickets.TicketOrigin, bool, error) {
		o, ok, err := git.LastCommitChangingLine(storeRoot, rel, "^status: cancelled$")
		return tickets.TicketOrigin{Path: o.Path, At: o.At, Root: o.Root}, ok, err
	}, dryRun)
	return mergeChanges(created, cancelled), err
}

// storeProjectEpics loads the epics of the ticket-store project at root. A root
// that is not a project of a git-tracked store has no history to read: it
// returns nil epics.
func storeProjectEpics(root string) (storeRoot string, epics []tickets.Epic, err error) {
	if _, err := config.ReadProjectFile(root); err != nil {
		return "", nil, nil
	}
	storeRoot = filepath.Dir(root)
	if _, err := os.Stat(filepath.Join(storeRoot, ".git")); err != nil {
		return "", nil, nil
	}
	all, err := tickets.LoadStoreEpics(storeRoot)
	if err != nil {
		return "", nil, err
	}
	project := tickets.ProjectName(root)
	for _, se := range all {
		if se.Project == project {
			epics = append(epics, se.Epic)
		}
	}
	return storeRoot, epics, nil
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

func ticketSessionTotals(ids []string) (int64, float64, error) {
	tokens, cost, err := ralphloop.SessionIDsTotals(ids)
	return int64(tokens), cost, err
}

// epicLogRoot is the directory holding an epic's run log: root for a live
// epic, its archive for an archived one.
func epicLogRoot(root, epicName string) string {
	if _, err := os.Stat(filepath.Join(root, epicName)); err != nil {
		if _, err := os.Stat(filepath.Join(tickets.ArchiveDir(root), epicName)); err == nil {
			return tickets.ArchiveDir(root)
		}
	}
	return root
}

// conflictSessionsUnder reads the sessions that resolved conflicts for a
// ticket from its epic's run log.
func conflictSessionsUnder(root string) func(epicName, ticketID string) ([]string, error) {
	return func(epicName, ticketID string) ([]string, error) {
		evs, _, err := ralphloop.ReadEvents(epicLogRoot(root, epicName), epicName)
		var ids []string
		for _, ev := range evs {
			if ev.Type == string(events.ConflictResolved) && ev.Ticket == ticketID &&
				ev.AgentSession != "" && !slices.Contains(ids, ev.AgentSession) {
				ids = append(ids, ev.AgentSession)
			}
		}
		return ids, err
	}
}

// earlyRunLogProject is the project that early run logs, written at
// <store>/<epic>/run-log.jsonl before projects had directories, belong to.
const earlyRunLogProject = "gx"

// ticketFrictionUnder builds a ticket's Friction summary from its epic's run log
// and its own log. The gx project falls back to an early run log in the store
// root, which only a project directory's own log supersedes.
func ticketFrictionUnder(root string) func(epicName string, t tickets.Ticket) (schema.FrictionSummary, error) {
	return func(epicName string, t tickets.Ticket) (schema.FrictionSummary, error) {
		runEvents, ok, err := ralphloop.ReadEvents(epicLogRoot(root, epicName), epicName)
		if err == nil && !ok && tickets.ProjectName(root) == earlyRunLogProject {
			runEvents, _, err = ralphloop.ReadEvents(filepath.Dir(root), epicName)
		}
		if err != nil {
			return schema.FrictionSummary{}, fmt.Errorf("run log of %s: %w", epicName, err)
		}
		log, err := tickets.ReadLog(t.FilePath())
		if err != nil {
			return schema.FrictionSummary{}, err
		}
		return ralphloop.Friction(t.Identifier, runEvents, log), nil
	}
}

// epicTrailerLinesUnder counts an epic's lines from the Ralph-Loop-Ticket
// trailer commits in the tracker root's repo. A merged epic's branch may be
// gone, so it falls back to the epic's target, where the commits landed. The
// repo is resolved on first use, like epicLandedUnder's. A root with no repo
// leaves lines unstamped rather than failing the migration.
func epicTrailerLinesUnder(root string) func(tickets.Epic) (int, int, bool, error) {
	var repoRoot, trunk string
	var resolved bool
	return func(e tickets.Epic) (int, int, bool, error) {
		if !resolved {
			resolved = true
			repoRoot, trunk, _ = trackerRepo(root)
		}
		if repoRoot == "" {
			return 0, 0, false, nil
		}
		ref := "refs/heads/" + e.Name
		if !git.RefExists(repoRoot, ref) {
			ref = e.Base
			if ref == "" {
				ref = trunk
			}
		}
		added, removed, err := ralphloop.EpicTrailerLines(repoRoot, ref, e.Name)
		return added, removed, err == nil, err
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
