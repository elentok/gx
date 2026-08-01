package git

import (
	"fmt"
	"strconv"
)

// RevParse resolves ref to its full commit hash in dir.
func RevParse(dir, ref string) (string, error) {
	out, _, err := run(dir, []string{"rev-parse", ref})
	return out, err
}

// MergeBase returns the best common ancestor of refA and refB (git
// merge-base). Unlike resolving a branch's tip at some point in time, this
// stays correct even after refB has advanced past the point refA branched
// from.
func MergeBase(dir, refA, refB string) (string, error) {
	out, _, err := run(dir, []string{"merge-base", refA, refB})
	return out, err
}

// CommitsAhead returns how many commits toRef has that fromExclusive
// doesn't (git rev-list --count fromExclusive..toRef) — zero means toRef has
// landed nothing new since fromExclusive.
func CommitsAhead(dir, fromExclusive, toRef string) (int, error) {
	out, _, err := run(dir, []string{"rev-list", "--count", fromExclusive + ".." + toRef})
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("invalid rev-list count %q: %w", out, err)
	}
	return n, nil
}

// IsAncestor reports whether ancestor is an ancestor of (or equal to)
// descendant (git merge-base --is-ancestor). Unlike MergeBase, it needs no
// second call to compare a computed common ancestor back against a specific
// commit — the common check behind confirming a commit that was cherry-
// picked elsewhere is actually reachable from a branch's current tip.
func IsAncestor(dir, ancestor, descendant string) (bool, error) {
	_, _, err := run(dir, []string{"merge-base", "--is-ancestor", ancestor, descendant})
	if err == nil {
		return true, nil
	}
	if runErr, ok := err.(*RunError); ok && runErr.Code == 1 {
		return false, nil
	}
	return false, err
}

// CherryPickRange cherry-picks the commit range fromExclusive..toInclusive
// (in order) onto dir's current branch via `git cherry-pick`. On a conflict,
// the returned error is a *RunError and dir is left mid-cherry-pick with
// conflict markers in place, same as running the command by hand.
func CherryPickRange(dir, fromExclusive, toInclusive string) error {
	_, _, err := run(dir, []string{"cherry-pick", fromExclusive + ".." + toInclusive})
	return err
}

// CherryPickInProgress reports whether dir has a cherry-pick sequence
// currently stopped on a conflict (CHERRY_PICK_HEAD present).
func CherryPickInProgress(dir string) (bool, error) {
	_, _, err := run(dir, []string{"rev-parse", "-q", "--verify", "CHERRY_PICK_HEAD"})
	if err != nil {
		if _, ok := err.(*RunError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
