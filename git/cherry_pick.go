package git

import (
	"fmt"
	"strconv"
	"strings"
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

// AbortCherryPick abandons the cherry-pick currently in progress in dir and
// restores the worktree to its pre-cherry-pick state. The source iteration
// branch remains untouched, so its commits can be landed again safely.
func AbortCherryPick(dir string) error {
	_, _, err := run(dir, []string{"cherry-pick", "--abort"})
	return err
}

// PatchesApplied reports whether every commit in base..branch already has a
// patch-equivalent commit reachable from upstream (git cherry, which
// compares patch-id rather than commit hash). Unlike IsAncestor, this stays
// correct even after upstream was rebased/amended past the point where these
// commits originally landed, since rebasing rewrites hashes but not patch
// content. An empty range (branch has no commits ahead of base) reports
// false — there's nothing to compare, so callers must treat that as
// "unverified", not "confirmed landed".
func PatchesApplied(dir, upstream, base, branch string) (bool, error) {
	out, _, err := run(dir, []string{"cherry", upstream, branch, base})
	if err != nil {
		return false, err
	}
	found := false
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		found = true
		if !strings.HasPrefix(line, "-") {
			return false, nil
		}
	}
	return found, nil
}

// Trailer is one "key: value" commit-message trailer line, see
// AppendTrailers.
type Trailer struct {
	Key, Value string
}

// AppendTrailer amends HEAD's commit message to add a "key: value" trailer
// line, leaving the tree/index untouched (RewordHead). Unlike a diff or
// hash, a trailer survives a later rebase even when the commit is manually
// re-resolved during a conflict (which changes its patch-id) — it's the
// last-resort marker classifyDoneTicket falls back to once both hash- and
// patch-id-based presence checks can no longer prove a ticket's landed
// commit is still on the feature branch.
func AppendTrailer(dir, key, value string) error {
	return AppendTrailers(dir, Trailer{Key: key, Value: value})
}

// AppendTrailers amends HEAD's commit message to add one or more "key:
// value" trailer lines in a single rewrite, rather than one amend per
// trailer — see AppendTrailer for why a trailer survives what a diff/hash
// can't.
func AppendTrailers(dir string, trailers ...Trailer) error {
	msg, _, err := run(dir, []string{"log", "-1", "--format=%B", "HEAD"})
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(msg, "\n"))
	b.WriteString("\n\n")
	for _, t := range trailers {
		b.WriteString(t.Key)
		b.WriteString(": ")
		b.WriteString(t.Value)
		b.WriteString("\n")
	}
	_, err = RewordHead(dir, b.String())
	return err
}

// TrailerCommitExists reports whether any commit reachable from ref carries
// a "key: value" trailer line in its message (git log --grep), see
// AppendTrailer.
func TrailerCommitExists(dir, ref, key, value string) (bool, error) {
	out, _, err := run(dir, []string{"log", "--format=%H", "--grep=^" + key + ": " + value + "$", ref})
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// TrailerMap parses every commit reachable from ref for a key trailer (git
// log --format='%H %(trailers:key=...,valueonly)') into a value→SHA map — a
// single shell-out replacing what would otherwise be one TrailerCommitExists
// call per value being looked up. A commit with no such trailer contributes
// nothing (the placeholder expands to an empty string, trimmed away here).
func TrailerMap(dir, ref, key string) (map[string]string, error) {
	out, _, err := run(dir, []string{"log", "--format=%H %(trailers:key=" + key + ",valueonly)", ref})
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for line := range strings.SplitSeq(out, "\n") {
		sha, value, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result[value] = sha
	}
	return result, nil
}

// CherryPickInProgress reports whether dir has a cherry-pick sequence
// currently stopped on a conflict or empty commit (CHERRY_PICK_HEAD present).
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
