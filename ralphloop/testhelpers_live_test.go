package ralphloop

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fakes"
)

// realGitDeps wires IsAncestor/RevParse/WorktreeExists to the real git
// package and filesystem, leaving every other Deps field faked.
func realGitDeps() deps.Deps {
	d, _, _ := fakes.FakeDeps()
	d.IsAncestor = git.IsAncestor
	d.RevParse = git.RevParse
	d.WorktreeExists = deps.DefaultDeps().WorktreeExists
	d.MergeBase = git.MergeBase
	d.PatchesApplied = history.PatchesApplied
	d.AppendTrailers = history.AppendTrailers
	return d
}

// fakeConflictErr stands in for the *git.RunError CherryPickRange returns on
// a real conflict; only its presence (not its type) matters to the merge path,
// which distinguishes conflicts from other errors via CherryPickInProgress.
type fakeConflictErr struct{}

func (e *fakeConflictErr) Error() string { return "cherry-pick conflict" }
