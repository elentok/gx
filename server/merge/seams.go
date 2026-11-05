package merge

import "github.com/elentok/gx/git"

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
