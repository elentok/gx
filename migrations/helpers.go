package migrations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketgit"
	"github.com/elentok/gx/tickets/ticketmigrate"
)

// designProbeUnder is the trunk checkout and repo of the project at root, for
// the design step guess. A root that is not a store project has neither.
func designProbeUnder(root string) ticketmigrate.DesignProbe {
	probe := ticketmigrate.DesignProbe{BranchExists: ticketgit.BranchExists}
	if _, err := config.ReadProjectFile(root); err != nil {
		return probe
	}
	store, project := filepath.Dir(root), tickets.ProjectName(root)
	probe.TrunkDir = ticketgit.TrunkCheckout(store, project)
	probe.Repo = ticketgit.ProjectRepoRoot(store, project)
	return probe
}

// backfillTicketCreation stamps created_at and the original address on the
// tickets of the ticket-store project at root from the store's git history
// (see ticketmigrate.BackfillTicketCreation). A root that is not a project of a
// git-tracked store has no history to read and is skipped.
func backfillTicketCreation(root string, dryRun bool) ([]tickets.MigrationChange, error) {
	storeRoot, epics, err := storeProjectEpics(root)
	if err != nil || epics == nil {
		return nil, err
	}
	created, err := ticketmigrate.BackfillTicketCreation(epics, storeRoot, ticketOrigin(func(rel string) (git.FileOrigin, bool, error) {
		return git.FirstCommitOfFile(storeRoot, rel)
	}), dryRun)
	if err != nil {
		return created, err
	}
	// Reload: the creation step may have rewritten tickets this one reads.
	_, epics, err = storeProjectEpics(root)
	if err != nil {
		return created, err
	}
	cancelled, err := ticketmigrate.BackfillCancelledAt(epics, storeRoot, ticketOrigin(func(rel string) (git.FileOrigin, bool, error) {
		return git.LastCommitChangingLine(storeRoot, rel, "^status: cancelled$")
	}), dryRun)
	return mergeChanges(created, cancelled), err
}

// ticketOrigin adapts a git history lookup to the origin func the backfills take.
func ticketOrigin(lookup func(rel string) (git.FileOrigin, bool, error)) func(string) (ticketmigrate.TicketOrigin, bool, error) {
	return func(rel string) (ticketmigrate.TicketOrigin, bool, error) {
		o, ok, err := lookup(rel)
		return ticketmigrate.TicketOrigin{Path: o.Path, At: o.At, Root: o.Root}, ok, err
	}
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
	all, err := ticketmigrate.LoadStoreEpics(storeRoot)
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

// epicLandedUnder reports whether an epic of the tracker root's repo has
// landed: its branch is gone, or is an ancestor of its target (its base,
// else trunk). The repo is resolved on first use, so a root with nothing to
// backfill never needs one. A root outside any repo has nothing that could
// have landed, so no epic counts as landed.
func epicLandedUnder(dirs config.Dirs, root string) func(tickets.Epic) (bool, error) {
	var repoRoot, trunk string
	noRepo := false
	return func(e tickets.Epic) (bool, error) {
		if noRepo {
			return false, nil
		}
		if repoRoot == "" {
			var err error
			repoRoot, trunk, err = trackerRepo(dirs, root)
			if errors.Is(err, git.ErrNoRepo) {
				noRepo = true
				return false, nil
			}
			if err != nil {
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
		evs, _, err := runlog.ReadEvents(epicLogRoot(root, epicName), epicName)
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
		runEvents, ok, err := runlog.ReadEvents(epicLogRoot(root, epicName), epicName)
		if err == nil && !ok && tickets.ProjectName(root) == earlyRunLogProject {
			runEvents, _, err = runlog.ReadEvents(filepath.Dir(root), epicName)
		}
		if err != nil {
			return schema.FrictionSummary{}, fmt.Errorf("run log of %s: %w", epicName, err)
		}
		log, err := tickets.ReadLog(t.FilePath())
		if err != nil {
			return schema.FrictionSummary{}, err
		}
		return ticketops.Friction(t.Identifier, runEvents, log), nil
	}
}

// epicTrailerLinesUnder counts an epic's lines from the Ralph-Loop-Ticket
// trailer commits in the tracker root's repo. A merged epic's branch may be
// gone, so it falls back to the epic's target, where the commits landed. The
// repo is resolved on first use, like epicLandedUnder's. A root with no repo
// leaves lines unstamped rather than failing the migration.
func epicTrailerLinesUnder(dirs config.Dirs, root string) func(tickets.Epic) (int, int, bool, error) {
	var repoRoot, trunk string
	var resolved bool
	return func(e tickets.Epic) (int, int, bool, error) {
		if !resolved {
			resolved = true
			repoRoot, trunk, _ = trackerRepo(dirs, root)
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
func trackerRepo(dirs config.Dirs, root string) (repoRoot, trunk string, err error) {
	if pf, err := config.ReadProjectFile(root); err == nil && pf.Repo != nil && *pf.Repo != "" {
		trunk, err := ticketgit.Trunk(root, *pf.Repo)
		return *pf.Repo, trunk, err
	}
	repo, err := ticketgit.OpenRepoDirs(dirs, root)
	if err != nil {
		return "", "", err
	}
	return repo.Root, repo.Trunk, nil
}
