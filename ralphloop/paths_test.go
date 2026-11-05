package ralphloop

import (
	"testing"

	"github.com/elentok/gx/ralphloop/worktree"
)

// A mid-flight split keeps the same Number across every letter (04 ->
// 04a/04b), so iterLabel/iterBranch must key off Identifier, not Number, or
// lettered siblings would collide on the same worktree/branch.
func TestIterLabelIterBranch_DistinctForLetteredSiblingsSharingNumber(t *testing.T) {
	t.Parallel()
	if worktree.IterationLabel("epic", "04a") == worktree.IterationLabel("epic", "04b") {
		t.Errorf("iterLabel(04a) == iterLabel(04b) = %q, want distinct labels", worktree.IterationLabel("epic", "04a"))
	}
	if worktree.IterationBranch("epic", "04a") == worktree.IterationBranch("epic", "04b") {
		t.Errorf("iterBranch(04a) == iterBranch(04b) = %q, want distinct branches", worktree.IterationBranch("epic", "04a"))
	}
}
