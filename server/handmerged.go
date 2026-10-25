package server

import (
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// stampHandMerged notices epics merged outside the server (gx merge or a plain
// git merge). Only a complete, unmerged epic with a queued root or a
// non-terminal ticket waiting on it is checked: the scope keeps idle cost flat
// as unmerged epics pile up under auto-ff-merge off. It reads the index, not
// the disk, so an out-of-scope epic costs no I/O at all.
func (s *Server) stampHandMerged() {
	queued := map[rootRef]bool{}
	for _, item := range s.queued.list() {
		if addr, err := parseQueuedAddress(item.Address); err == nil {
			queued[rootOf(addr)] = true
		}
	}
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		epics, ok := s.idx.epicsOf(dir)
		if !ok {
			continue
		}
		var waited map[string]bool
		for _, e := range epics {
			if e.IsMap || e.CompletedAt.IsZero() || !e.MergedAt.IsZero() {
				continue
			}
			root := rootRef{Project: project, Epic: e.Name}
			if !queued[root] {
				if waited == nil {
					waited = tickets.EpicsWaitedOn(project, epics)
				}
				if !waited[e.Name] {
					continue
				}
			}
			s.stampIfMerged(root, epics, e)
		}
	}
}

// stampIfMerged stamps merged_at on e once its branch is an ancestor of its
// target, and dequeues its root if one is still queued (a root pause). It
// re-reads the stamp under the trunk's lock, so a server merge racing it sends
// the only notice. A held lock skips this tick: a poll never queues behind a
// merge, and the next tick checks again.
func (s *Server) stampIfMerged(root rootRef, epics []tickets.Epic, e tickets.Epic) {
	dir, repo, err := s.projectOf(root.Project)
	if err != nil {
		return
	}
	t, _, target, err := s.epicRefs(root.Project, epics, e.Name, repo)
	if err != nil {
		return
	}
	unlock, ok := s.trunkLocks.tryLock(repo, target)
	if !ok {
		return
	}
	removed := s.stampMerged(root, e, repo, target)
	unlock()
	if removed {
		s.notifyEpicOutcome(root.Project, dir, ralphloop.OneIteration{Epic: e.Name, Ticket: t}, e, true)
	}
}

// stampMerged is stampIfMerged under the trunk lock. It reports whether it
// dequeued the root, the one case that sends a notice.
func (s *Server) stampMerged(root rootRef, e tickets.Epic, repo, target string) (removed bool) {
	if !tickets.LoadEpicTicketMD(e.Path).MergedAt.IsZero() {
		return false
	}
	if merged, err := s.mergedProbe(repo, "refs/heads/"+e.Name, target); err != nil || !merged {
		return false
	}
	added, deleted, err := ralphloop.EpicTrailerLines(repo, "refs/heads/"+e.Name, e.Name)
	s.stampMergedEpic(root, e, added, deleted, err)
	s.stampEpic(root, e.Path, "merged_at")
	s.log.Info("epic merged outside the server", "root", root, "target", target)
	removed, err = s.queued.removeRoot(root)
	if err != nil {
		s.log.Warn("dequeue merged root", "root", root, "err", err)
		return false
	}
	if removed {
		s.events.publish(EventQueueChanged, root.String())
		s.events.publish(EventRootCompleted, root.String())
	}
	return removed
}
