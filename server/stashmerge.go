package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// epicStashMerge is the "Stash, merge and restore" verb for an epic paused
// dirty-trunk: it backs up the clashing uncommitted files, fast-forwards the
// target and puts the files back with conflict markers, committing nothing. A
// trunk-conflict ticket carries the resolution; until it is done the epic is
// not merged.
func (s *Server) epicStashMerge(req QueueRequest) (QueueResult, error) {
	m, refused, err := s.loadMergeEpic(req)
	if refused != nil || err != nil {
		return refusedOr(refused), err
	}
	if m.epic.MergeBlockKind != string(events.DirtyTrunk) {
		return refusal(ReasonNotDirtyTrunk, m.root.String()+" is not paused on a dirty trunk"), nil
	}
	first, _, target, err := s.epicRefs(m.root.Project, m.epics, m.root.Epic, m.repo)
	if err != nil {
		return QueueResult{}, err
	}
	one := ralphloop.OneIteration{Epic: m.root.Epic, Ticket: first}
	unlock := s.trunkLocks.lock(m.repo, target)
	res, d, err := s.stashMergeLocked(m, one, target)
	unlock()
	if err != nil {
		return QueueResult{}, err
	}
	// The files restored cleanly: this was an ordinary merge after all.
	if d != nil {
		return s.actOnMerge(m.root, one, m.dir, m.epic, *d), nil
	}
	return res, nil
}

// stashMergeLocked is epicStashMerge's work under the target's trunk lock. A
// decision is returned when the clashing files restore without conflict, so the
// caller finishes an ordinary merge instead.
func (s *Server) stashMergeLocked(m mergeEpicRef, one ralphloop.OneIteration, target string) (QueueResult, *mergeDecision, error) {
	root, e, epic := m.root, m.epic, m.root.Epic
	if r := epicMergeRefusal(root, e, m.repo); r != nil {
		return *r, nil, nil
	}
	info, err := git.IdentifyDir(m.repo)
	if err != nil {
		return QueueResult{}, nil, err
	}
	worktrees, err := git.ListWorktrees(info.Repo)
	if err != nil {
		return QueueResult{}, nil, err
	}
	dir := git.CheckoutDirForBranch(info.Repo, worktrees, target)
	if dir == "" {
		return QueueResult{}, nil, fmt.Errorf("no checkout of %s", target)
	}
	if ff, err := git.IsAncestor(m.repo, target, "refs/heads/"+epic); err != nil {
		return QueueResult{}, nil, err
	} else if !ff {
		return refusal(ReasonNeedsRebase, fmt.Sprintf("%s is not a fast-forward of %s; merge again to rebase it", epic, target)), nil, nil
	}
	// Recomputed now: the files may have changed since the pause.
	clashing := clashingFiles(info.Repo.Root, dir, target, epic)
	if len(clashing) == 0 {
		return refusal(ReasonNotDirtyTrunk, "no uncommitted file on "+target+" clashes any more; merge again"), nil, nil
	}
	snapshot, err := git.SnapshotPaths(dir, clashing)
	if err != nil {
		return QueueResult{}, nil, err
	}
	tree, clean, err := git.MergeSnapshotMarked(dir, snapshot, "refs/heads/"+epic)
	if err != nil {
		return QueueResult{}, nil, err
	}
	if clean {
		d, err := s.epicMergeLocked(root, one, m.repo, m.epics, e, target)
		return QueueResult{}, &d, err
	}
	ref, now, err := s.backupSnapshot(info.Repo.Root, epic, snapshot)
	if err != nil {
		return QueueResult{}, nil, err
	}
	if err := git.SetAside(dir, clashing); err != nil {
		return QueueResult{}, nil, fmt.Errorf("set aside %s: %w (backup %s)", listFiles(clashing), err, ref)
	}
	ok, err := git.MergeFastForward(dir, epic)
	if err != nil || !ok {
		if rerr := git.RestoreFromTree(dir, snapshot, clashing); rerr != nil {
			s.log.Warn("restore after failed stash-merge", "epic", epic, "backup", ref, "err", rerr)
		}
		if err == nil {
			err = fmt.Errorf("%s is no longer a fast-forward of %s", epic, target)
		}
		return QueueResult{}, nil, err
	}
	if err := git.RestoreFromTree(dir, tree, clashing); err != nil {
		return QueueResult{}, nil, fmt.Errorf("fast-forwarded, but restoring %s failed: %w (backup %s)", listFiles(clashing), err, ref)
	}
	return s.recordTrunkConflict(m, one, target, ref, clashing, now)
}

// recordTrunkConflict writes down a stash-merge that left conflict markers: the
// epic fields, the run log and the trunk-conflict ticket. The root gets no
// merged_at and no completed_at, so it stays queued and dependent epics wait.
func (s *Server) recordTrunkConflict(m mergeEpicRef, one ralphloop.OneIteration, target, ref string, files []string, now time.Time) (QueueResult, *mergeDecision, error) {
	root, e := m.root, m.epic
	err := tickets.StampEpicFields(e.Path, map[string]any{
		"trunk_fast_forwarded_at": now.UTC(),
		"trunk_conflict_backup":   ref,
		"trunk_conflict_files":    files,
	})
	if err != nil {
		return QueueResult{}, nil, fmt.Errorf("stamp %s: %w", root, err)
	}
	s.clearMergeBlock(root, e.Path)
	if err := tickets.ClearEpicFields(e.Path, "completed_at"); err != nil {
		return QueueResult{}, nil, fmt.Errorf("clear completed_at of %s: %w", root, err)
	}
	_ = ralphloop.AppendEvent(m.dir, root.Epic, ralphloop.Event{
		Time: now, Type: string(events.TrunkConflictOpened), Reason: strings.Join(files, ", "), BackupRef: ref,
	})
	_, id, err := writeNewTicket(e.Path, "", "trunk-conflict", func(id string) (schema.Ticket, string) {
		tk := schema.Ticket{ID: schema.TicketID(id), Status: schema.StatusOpen, Type: schema.TypeTrunkConflict}
		body := fmt.Sprintf("\n# %s — Resolve the conflicts on %s\n\n## What to build\n\n"+
			"\"Stash, merge and restore\" fast-forwarded %s to %s and put your uncommitted changes back. These files hold conflict markers in the checkout of %s and nothing is committed:\n\n%s\n\n"+
			"Your changes before the merge are kept in the backup ref `%s`.\n",
			id, target, target, root.Epic, target, bulletList(files), ref)
		return tk, body
	})
	if err != nil {
		return QueueResult{}, nil, fmt.Errorf("add trunk-conflict ticket to %s: %w", root, err)
	}
	if err := s.ensureRootQueued(root, one.Ticket); err != nil {
		s.log.Warn("queue root after stash-merge", "root", root, "err", err)
	}
	s.log.Info("stash-merge left conflicts", "root", root, "ticket", id, "backup", ref)
	s.events.publish(EventQueueChanged, root.String())
	return QueueResult{TrunkConflict: tickets.Address{Project: root.Project, Epic: root.Epic, ID: id}.String()}, nil, nil
}

func bulletList(items []string) string {
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = "- `" + it + "`"
	}
	return strings.Join(lines, "\n")
}
