package ralphloop

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

// An agent that ran `git checkout -q HEAD~0` and then committed leaves the
// iteration branch at its base; the commit on the detached HEAD still counts,
// so the finish lands it instead of parking it as a zero-commit finish.
func TestIterationCommitsAhead_CountsCommitsOnDetachedHead(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	testutil.MustGitExported(t, dir, "checkout", "-q", "-b", "ralph-loop/epic-item-04")
	testutil.MustGitExported(t, dir, "checkout", "-q", "HEAD~0")
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")

	ahead, err := iterationCommitsAhead(testDeps(), dir, base, "ralph-loop/epic-item-04")
	if err != nil {
		t.Fatalf("iterationCommitsAhead: %v", err)
	}
	if ahead != 1 {
		t.Errorf("iterationCommitsAhead() = %d, want 1", ahead)
	}
}
