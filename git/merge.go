package git

// MergeOutcome is the result of MergeBranchInto. Merged is false when branch
// needs a rebase; Branch, Target and WorktreePath are then set for the report.
type MergeOutcome struct {
	Merged       bool
	Branch       string
	Target       string
	WorktreePath string
}

// MergeBranchInto is the deterministic core of `gx merge` (ADR 0015): a
// fast-forward-only merge of branch into target, run in target's own worktree.
// It never rebases, never pushes, and stops without further mutation on a
// non-fast-forward refusal.
//
// Without a worktree on target it falls back to repo.Root; for a non-bare repo
// that's the only working tree there is, and for a bare repo it has none, so
// the merge fails with git's own "must be run in a work tree" error.
func MergeBranchInto(repo Repo, worktrees []Worktree, branch, target string) (MergeOutcome, error) {
	dir := WorktreePathForBranch(target, worktrees)
	if dir == "" {
		dir = repo.Root
	}
	ok, err := MergeFastForward(dir, branch)
	if err != nil {
		return MergeOutcome{}, err
	}
	if ok {
		return MergeOutcome{Merged: true}, nil
	}
	return MergeOutcome{Branch: branch, Target: target, WorktreePath: WorktreePathForBranch(branch, worktrees)}, nil
}

// WorktreePathForBranch returns the path of the worktree checked out to
// branch, or "" when there is none.
func WorktreePathForBranch(branch string, worktrees []Worktree) string {
	for _, wt := range worktrees {
		if wt.Branch == branch {
			return wt.Path
		}
	}
	return ""
}

// MergeFastForward runs `git merge --ff-only branch` in dir. ok is true on
// success. A refusal because branch is not a fast-forward of dir's current
// HEAD is an expected outcome, not a failure: it's reported as ok=false,
// err=nil. Fast-forwardability is decided up front via IsAncestor rather
// than by matching merge stderr, since git localizes that text under
// non-English LC_ALL/LANG. Any other failure is returned as err.
func MergeFastForward(dir, branch string) (ok bool, err error) {
	ancestor, err := IsAncestor(dir, "HEAD", branch)
	if err != nil {
		return false, err
	}
	if !ancestor {
		return false, nil
	}
	if _, _, err := run(dir, []string{"merge", "--ff-only", branch}); err != nil {
		return false, err
	}
	return true, nil
}
