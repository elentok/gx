package git

import (
	"errors"
	"fmt"
)

// MergeOutcome is the result of MergeBranchInto. Merged is false when branch
// needs a rebase; Branch, Target and WorktreePath are then set for the report.
type MergeOutcome struct {
	Merged       bool
	Branch       string
	Target       string
	WorktreePath string
}

// MergeBranchInto is the deterministic core of `gx merge` (ADR 0015): a
// fast-forward-only merge of branch into target. It never rebases, never
// pushes, never moves a branch other than target, and stops without further
// mutation on a non-fast-forward refusal or a missing target.
//
// Where target is checked out (a worktree, or a non-bare root's HEAD) the
// merge runs there so the working tree follows. Otherwise the ref is moved
// directly: merging in the root would advance whatever branch it has checked
// out instead.
func MergeBranchInto(repo Repo, worktrees []Worktree, branch, target string) (MergeOutcome, error) {
	targetSHA, err := RevParse(repo.Root, "refs/heads/"+target)
	if errors.Is(err, ErrRefNotFound) {
		return MergeOutcome{}, fmt.Errorf("target branch %q does not exist", target)
	}
	if err != nil {
		return MergeOutcome{}, err
	}

	var ok bool
	if dir := checkoutDirForBranch(repo, worktrees, target); dir != "" {
		ok, err = MergeFastForward(dir, branch)
	} else {
		ok, err = fastForwardRef(repo.Root, target, targetSHA, branch)
	}
	if err != nil {
		return MergeOutcome{}, err
	}
	if ok {
		return MergeOutcome{Merged: true}, nil
	}
	return MergeOutcome{Branch: branch, Target: target, WorktreePath: WorktreePathForBranch(branch, worktrees)}, nil
}

// checkoutDirForBranch returns the working tree that has branch checked out:
// a linked worktree, else a non-bare root whose HEAD is branch, else "".
func checkoutDirForBranch(repo Repo, worktrees []Worktree, branch string) string {
	if dir := WorktreePathForBranch(branch, worktrees); dir != "" {
		return dir
	}
	if repo.IsBare {
		return ""
	}
	if head, err := CurrentBranch(repo.Root); err == nil && head == branch {
		return repo.Root
	}
	return ""
}

// fastForwardRef moves refs/heads/target from oldSHA to branch's tip without
// touching any worktree. ok is false when target is not an ancestor of
// branch. Passing oldSHA to update-ref makes a concurrent move of target fail
// instead of being overwritten.
func fastForwardRef(dir, target, oldSHA, branch string) (ok bool, err error) {
	ancestor, err := IsAncestor(dir, oldSHA, branch)
	if err != nil || !ancestor {
		return false, err
	}
	newSHA, err := RevParse(dir, branch)
	if err != nil {
		return false, err
	}
	if _, _, err := run(dir, []string{"update-ref", "refs/heads/" + target, newSHA, oldSHA}); err != nil {
		return false, err
	}
	return true, nil
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
