package git

import (
	"errors"
	"fmt"
	"github.com/elentok/gx/git/internal/run"
	"strconv"
	"strings"
)

// ErrRefNotFound is returned by RevParse when ref does not resolve, as opposed
// to git itself failing.
var ErrRefNotFound = errors.New("ref not found")

// RevParse resolves ref to its full commit hash in dir. A ref that does not
// exist returns ErrRefNotFound; any other failure is returned as is.
func RevParse(dir, ref string) (string, error) {
	// --verify --quiet exits 1 with no output for a missing ref; real git
	// failures (bad repo, corrupt object) exit 128.
	out, _, err := run.Exec(dir, []string{"rev-parse", "--verify", "--quiet", ref})
	var runErr *RunError
	if errors.As(err, &runErr) && runErr.Code == 1 {
		return "", fmt.Errorf("%w: %s", ErrRefNotFound, ref)
	}
	return out, err
}

// MergeBase returns the best common ancestor of refA and refB (git
// merge-base). Unlike resolving a branch's tip at some point in time, this
// stays correct even after refB has advanced past the point refA branched
// from.
func MergeBase(dir, refA, refB string) (string, error) {
	out, _, err := run.Exec(dir, []string{"merge-base", refA, refB})
	return out, err
}

// CommitSubjects returns the subjects of fromExclusive..toRef, oldest first.
func CommitSubjects(dir, fromExclusive, toRef string) ([]string, error) {
	out, _, err := run.Exec(dir, []string{"log", "--reverse", "--format=%s", fromExclusive + ".." + toRef})
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// UnreachableCommits lists the commits on dir's HEAD that no branch, remote or
// tag reaches, oldest first, as "<short hash> <subject>" — what a detached
// worktree's removal would leave dangling.
func UnreachableCommits(dir string) ([]string, error) {
	out, _, err := run.Exec(dir, []string{"log", "--reverse", "--format=%h %s", "HEAD", "--not", "--branches", "--remotes", "--tags"})
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// CommitsAhead returns how many commits toRef has that fromExclusive
// doesn't (git rev-list --count fromExclusive..toRef) — zero means toRef has
// landed nothing new since fromExclusive.
func CommitsAhead(dir, fromExclusive, toRef string) (int, error) {
	out, _, err := run.Exec(dir, []string{"rev-list", "--count", fromExclusive + ".." + toRef})
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("invalid rev-list count %q: %w", out, err)
	}
	return n, nil
}

// AttachDetachedHead puts dir's HEAD back on branch when HEAD is detached at a
// commit branch leads to (a `git checkout HEAD~0` detaches it), moving branch
// forward to HEAD so commits made while detached count as branch's own. It
// reports whether it moved branch; an attached HEAD, or one branch does not
// lead to, is left alone.
func AttachDetachedHead(dir, branch string) (bool, error) {
	_, _, err := run.Exec(dir, []string{"symbolic-ref", "-q", "HEAD"})
	if err == nil {
		return false, nil
	}
	if runErr, ok := err.(*RunError); !ok || runErr.Code != 1 {
		return false, err
	}
	ahead, err := IsAncestor(dir, branch, "HEAD")
	if err != nil || !ahead {
		return false, err
	}
	if _, _, err := run.Exec(dir, []string{"switch", "-q", "-C", branch}); err != nil {
		return false, err
	}
	return true, nil
}

// IsAncestor reports whether ancestor is an ancestor of (or equal to)
// descendant (git merge-base --is-ancestor). Unlike MergeBase, it needs no
// second call to compare a computed common ancestor back against a specific
// commit — the common check behind confirming a commit that was cherry-
// picked elsewhere is actually reachable from a branch's current tip.
func IsAncestor(dir, ancestor, descendant string) (bool, error) {
	_, _, err := run.Exec(dir, []string{"merge-base", "--is-ancestor", ancestor, descendant})
	if err == nil {
		return true, nil
	}
	if runErr, ok := err.(*RunError); ok && runErr.Code == 1 {
		return false, nil
	}
	return false, err
}

// DiffNames lists the paths whose content differs between the tips of refA and
// refB (git diff --name-only refA refB).
func DiffNames(dir, refA, refB string) ([]string, error) {
	out, _, err := run.Exec(dir, []string{"diff", "--name-only", refA, refB})
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// DiffLines counts the lines added and removed going from refA to refB. A
// binary file has no line count and adds nothing.
func DiffLines(dir, refA, refB string) (added, removed int, err error) {
	out, _, err := run.Exec(dir, []string{"diff", "--numstat", refA, refB})
	if err != nil {
		return 0, 0, err
	}
	added, removed = sumNumstat(out)
	return added, removed, nil
}

// sumNumstat is shared with TrailerLines, so it stays in core.
func sumNumstat(out string) (added, removed int) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if n, err := strconv.Atoi(fields[0]); err == nil {
			added += n
		}
		if n, err := strconv.Atoi(fields[1]); err == nil {
			removed += n
		}
	}
	return added, removed
}
