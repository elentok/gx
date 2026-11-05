package merge

import (
	"errors"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/tickets"
)

// CompleteRootOf runs the all-done check of address's root as if its
// iteration had just settled. For tests.
func (s *Merger) CompleteRootOf(address string) error {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	_, t, ok := s.host.FreshTicket(addr)
	if !ok {
		return errors.New("no ticket " + address)
	}
	s.CompleteRootIfDone(queue.RootOf(addr), ralphloop.OneIteration{Epic: addr.Epic, Ticket: t})
	return nil
}

// RemoveMergedWorktreeSettled starts the post-merge worktree removal the way
// stampEpic does, on the settle group. For tests.
func (s *Reconciler) RemoveMergedWorktreeSettled(project, epic string) {
	root := queue.RootRef{Project: project, Epic: epic}
	s.guards.GoSettle(func() { s.RemoveMergedWorktree(root) })
}

// SetMergeInto swaps the merge core a root merge calls. For tests.
func (s *Merger) SetMergeInto(f func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error)) {
	s.mergeInto = f
}

// SetMergedProbe swaps the poll-time ancestry check. For tests.
func (s *Reconciler) SetMergedProbe(f func(repo, ancestor, descendant string) (bool, error)) {
	s.mergedProbe = f
}

// HoldTrunkLock takes the project's trunk lock the way a merge does, until
// release runs. For tests.
func (s *Merger) HoldTrunkLock(project, trunk string) (release func(), err error) {
	_, repo, err := s.projects.Of(project)
	if err != nil {
		return nil, err
	}
	return s.guards.Trunks.Lock(repo, trunk), nil
}

// LockedTrunks lists the trunks any merge has locked so far, in any repo. For
// tests.
func (s *Merger) LockedTrunks() []string {
	return s.guards.Trunks.Locked()
}
