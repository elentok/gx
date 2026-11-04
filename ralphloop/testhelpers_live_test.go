package ralphloop

import (
	"path/filepath"
	"time"

	"github.com/elentok/gx/git"
)

// readyTimer is a Deps.PauseTimer that has already fired: a pause test polls at
// full speed instead of waiting out a real interval.
func readyTimer(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

func ticketPath(scratchDir, epicName, file string) string {
	return filepath.Join(scratchDir, epicName, "issues", file)
}

// realGitDeps wires IsAncestor/RevParse/WorktreeExists to the real git
// package and filesystem, leaving every other Deps field faked.
func realGitDeps() Deps {
	d, _, _ := fakeDeps()
	d.IsAncestor = git.IsAncestor
	d.RevParse = git.RevParse
	d.WorktreeExists = worktreeExists
	d.MergeBase = git.MergeBase
	d.PatchesApplied = git.PatchesApplied
	d.AppendTrailers = git.AppendTrailers
	return d
}

// fakeConflictErr stands in for the *git.RunError CherryPickRange returns on
// a real conflict; only its presence (not its type) matters to the merge path,
// which distinguishes conflicts from other errors via CherryPickInProgress.
type fakeConflictErr struct{}

func (e *fakeConflictErr) Error() string { return "cherry-pick conflict" }
