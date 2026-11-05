package git

import (
	"github.com/elentok/gx/git/internal/run"
	"os"
	"path/filepath"
	"strings"
)

// TrailerLines sums the lines added and removed by the commits reachable from
// ref whose key trailer value satisfies match. Other commits in ref's history
// count for nothing.
func TrailerLines(dir, ref, key string, match func(value string) bool) (added, removed int, err error) {
	trailers, err := TrailerMap(dir, ref, key)
	if err != nil {
		return 0, 0, err
	}
	for value, sha := range trailers {
		if !match(value) {
			continue
		}
		out, _, err := run.Exec(dir, []string{"show", "--numstat", "--format=", sha})
		if err != nil {
			return 0, 0, err
		}
		a, r := sumNumstat(out)
		added += a
		removed += r
	}
	return added, removed, nil
}

// CherryPickRange cherry-picks the commit range fromExclusive..toInclusive
// (in order) onto dir's current branch via `git cherry-pick`. On a conflict,
// the returned error is a *RunError and dir is left mid-cherry-pick with
// conflict markers in place, same as running the command by hand.
func CherryPickRange(dir, fromExclusive, toInclusive string) error {
	_, _, err := run.Exec(dir, []string{"cherry-pick", fromExclusive + ".." + toInclusive})
	return err
}

// AbortCherryPick abandons the cherry-pick currently in progress in dir and
// restores the worktree to its pre-cherry-pick state. The source iteration
// branch remains untouched, so its commits can be landed again safely.
func AbortCherryPick(dir string) error {
	_, _, err := run.Exec(dir, []string{"cherry-pick", "--abort"})
	return err
}

// AbortRebase abandons the rebase currently in progress in dir and puts the
// branch back at its pre-rebase tip.
func AbortRebase(dir string) error {
	_, _, err := run.Exec(dir, []string{"rebase", "--abort"})
	return err
}

// PatchesApplied reports whether every commit in base..branch already has a
// patch-equivalent commit reachable from upstream (git cherry, which
// compares patch-id rather than commit hash). Unlike IsAncestor, this stays
// correct even after upstream was rebased/amended past the point where these
// commits originally landed, since rebasing rewrites hashes but not patch
// content. git cherry omits commits upstream already reaches, so a range
// that is itself on upstream (e.g. a landed commit's own X^..X) prints
// nothing yet is applied: it reports true. An empty range (branch has no
// commits ahead of base) reports false — there's nothing to compare, so
// callers must treat that as "unverified", not "confirmed landed".
func PatchesApplied(dir, upstream, base, branch string) (bool, error) {
	total, missing, err := cherry(dir, upstream, branch, base)
	if err != nil {
		return false, err
	}
	if len(missing) > 0 {
		return false, nil
	}
	if total > 0 {
		return true, nil
	}
	ahead, err := CommitsAhead(dir, base, branch)
	return ahead > 0, err
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
	msg, _, err := run.Exec(dir, []string{"log", "-1", "--format=%B", "HEAD"})
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
	out, _, err := run.Exec(dir, []string{"log", "--format=%H", "--grep=^" + key + ": " + value + "$", ref})
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
	out, _, err := run.Exec(dir, []string{"log", "--format=%H %(trailers:key=" + key + ",valueonly)", ref})
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
// currently stopped on a conflict or empty commit. This checks both
// CHERRY_PICK_HEAD and the on-disk sequencer directory: an abort that didn't
// run to completion (or was interrupted) can clear CHERRY_PICK_HEAD while
// leaving sequencer/todo behind, and git itself still refuses to start a new
// cherry-pick in that state ("cherry-pick is already in progress") even
// though a CHERRY_PICK_HEAD-only check reports nothing in progress.
func CherryPickInProgress(dir string) (bool, error) {
	_, _, err := run.Exec(dir, []string{"rev-parse", "-q", "--verify", "CHERRY_PICK_HEAD"})
	if err == nil {
		return true, nil
	}
	if _, ok := err.(*RunError); !ok {
		return false, err
	}

	gitDir, _, err := run.Exec(dir, []string{"rev-parse", "--git-dir"})
	if err != nil {
		return false, err
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	if _, err := os.Stat(filepath.Join(gitDir, "sequencer", "todo")); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return false, nil
}

// RebaseInProgress reports whether dir's worktree is stopped mid-rebase, by
// either rebase backend.
func RebaseInProgress(dir string) (bool, error) {
	gitDir, _, err := run.Exec(dir, []string{"rev-parse", "--git-dir"})
	if err != nil {
		return false, err
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	for _, state := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(gitDir, state)); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
	}
	return false, nil
}

// UnappliedPatches returns the commits of head whose patch is not in upstream
// (git cherry), oldest first. It compares patches, so a rebased upstream still
// counts its commits as applied.
func UnappliedPatches(dir, upstream, head string) ([]string, error) {
	_, missing, err := cherry(dir, upstream, head)
	return missing, err
}

// cherry runs git cherry with args (upstream, head and an optional limit) and
// returns how many commits it listed and the shas whose patch is not upstream.
func cherry(dir string, args ...string) (total int, missing []string, err error) {
	out, _, err := run.Exec(dir, append([]string{"cherry"}, args...))
	if err != nil {
		return 0, nil, err
	}
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		total++
		if sha, ok := strings.CutPrefix(line, "+ "); ok {
			missing = append(missing, sha)
		}
	}
	return total, missing, nil
}
