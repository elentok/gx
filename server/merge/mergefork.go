package merge

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// maxMergeTickets is how many merge tickets one run gets before the root pauses.
const maxMergeTickets = 3

// mergeTicketsThisRun counts e's merge tickets from the latest manual one
// onward, that one included; all of them when none is manual. The store is the
// count's only record, so a restart keeps it.
func mergeTicketsThisRun(e tickets.Epic) int {
	merges := make([]tickets.Ticket, 0)
	for _, t := range e.Tickets {
		if t.Type == string(schema.TypeMerge) {
			merges = append(merges, t)
		}
	}
	slices.SortFunc(merges, func(a, b tickets.Ticket) int {
		return cmp.Or(cmp.Compare(a.Number, b.Number), cmp.Compare(a.Identifier, b.Identifier))
	})
	for i := len(merges) - 1; i >= 0; i-- {
		if merges[i].Manual {
			return len(merges) - i
		}
	}
	return len(merges)
}

// hasManualMergeTicket reports whether e holds a merge ticket the manual
// trigger forked. Unmerged, such an epic merges whatever auto-ff-merge says:
// the rule reads the store, so a retry ticket finishing last still merges.
func hasManualMergeTicket(e tickets.Epic) bool {
	return slices.ContainsFunc(e.Tickets, func(t tickets.Ticket) bool {
		return t.Type == string(schema.TypeMerge) && t.Manual
	})
}

// prMergeBody is the body of a PR-mode merge ticket: the branch is on GitHub,
// so the ticket rebases onto the remote branch first (keeping foreign commits),
// then onto trunk, and pushes with a lease. Nothing merges locally afterwards.
func prMergeBody(id, epic, target string) string {
	return fmt.Sprintf(`
# %[1]s — Rebase %[2]s onto %[3]s for its pull request

## What to build

The pull request of %[2]s conflicts with %[3]s. Fix that on the epic's branch; a person merges the pull request on GitHub, so nothing merges locally.

1. `+"`git fetch origin`"+`
2. `+"`git rebase origin/%[2]s`"+`: keep every commit someone else pushed to the remote branch.
3. `+"`git rebase origin/%[3]s`"+`: resolve the conflicts with %[3]s.
4. `+"`git push --force-with-lease origin %[2]s`"+`. Never a plain force.

If step 2 conflicts with commits you did not make, run `+"`git rebase --abort`"+`, push nothing, and write a Resolution that has the line `+"`Outcome: foreign-conflict`"+` and says what conflicts.
`, id, epic, target)
}

// ForkPRMerge forks a PR-mode merge ticket for root's conflicting pull request
// and queues it, unless a merge ticket is already pending. At the cap it forks
// none and stores "needs rebase" instead.
func (s *Merger) ForkPRMerge(root queue.RootRef) error {
	dir, repo, err := s.projects.Of(root.Project)
	if err != nil {
		return err
	}
	epics, err := tickets.LoadResolved(root.Project, dir)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return filepath.Base(e.Path) == root.Epic })
	if i < 0 {
		return fmt.Errorf("no epic %s", root)
	}
	e := epics[i]
	if slices.ContainsFunc(e.Tickets, func(t tickets.Ticket) bool { return pendingMerge(e, t) }) {
		return nil
	}
	_, _, target, err := s.EpicRefs(root.Project, epics, root.Epic, repo)
	if err != nil {
		return err
	}
	id := s.forkMergeTicket(root, e, target, false)
	if id == "" {
		if mergeTicketsThisRun(e) < maxMergeTickets {
			return fmt.Errorf("fork merge ticket for %s", root)
		}
		s.PauseEpic(root, e, events.NeedsRebase, fmt.Sprintf("needs rebase: the pull request of %s still conflicts with %s after %d merge tickets; rebase it, then merge again", root.Epic, target, maxMergeTickets))
		return nil
	}
	return s.EnsureQueued(root, id)
}

// forkMergeTicket forks a merge ticket into e for its non-ff onto target and
// returns its id. At the cap, or when the write fails, it forks none and the
// caller pauses the root. A manual ticket, the manual trigger's, skips the cap.
func (s *Merger) forkMergeTicket(root queue.RootRef, e tickets.Epic, target string, manual bool) string {
	if !manual && mergeTicketsThisRun(e) >= maxMergeTickets {
		return ""
	}
	_, id, err := ticketwrite.WriteNewTicket(e.Path, "", "merge", func(id string) (schema.Ticket, string) {
		tk := schema.Ticket{ID: schema.TicketID(id), Status: schema.StatusOpen, Type: schema.TypeMerge, Manual: manual}
		body := fmt.Sprintf("\n# %s — Merge %s onto %s\n\n## What to build\n\nRebase %s onto %s so it can fast-forward. The server merges it once this ticket is done.\n",
			id, root.Epic, target, root.Epic, target)
		if e.PRNumber != 0 {
			body = prMergeBody(id, root.Epic, target)
		}
		return tk, body
	})
	if err != nil {
		s.log.Warn("fork merge ticket", "root", root, "err", err)
		return ""
	}
	s.log.Info("merge ticket forked", "root", root, "ticket", id, "manual", manual)
	s.events.Publish(wire.EventQueueChanged, root.String())
	return id
}
