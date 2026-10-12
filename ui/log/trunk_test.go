package log

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

// newTrunkRepo returns a repo registered with trunk where "feature" is one
// commit ahead of "develop", which is one commit ahead of main.
func newTrunkRepo(t *testing.T, trunk string) string {
	t.Helper()
	repoDir := testutil.TempRepo(t)
	for _, name := range []string{"develop", "feature"} {
		testutil.MustGitExported(t, repoDir, "checkout", "-b", name)
		testutil.WriteFile(t, repoDir, name+".txt", name)
		testutil.CommitAll(t, repoDir, name)
	}
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	testutil.RegisterProject(t, repo.Root, trunk)
	return repoDir
}

func TestFetchBranchHistoryClasses_FollowsTrunk(t *testing.T) {
	for trunk, want := range map[string]int{"develop": 1, "main": 2} {
		t.Run(trunk, func(t *testing.T) {
			repoDir := newTrunkRepo(t, trunk)
			classes, _, trunkErr := fetchBranchHistoryClasses(repoDir, "HEAD")
			if trunkErr != nil {
				t.Fatalf("trunkErr = %v", trunkErr)
			}
			if len(classes) != want {
				t.Fatalf("got %d commits since %s, want %d", len(classes), trunk, want)
			}
		})
	}
}

func TestFetchBranchHistoryClasses_MissingTrunkWarnsWithoutFallback(t *testing.T) {
	repoDir := newTrunkRepo(t, "nope")
	classes, _, trunkErr := fetchBranchHistoryClasses(repoDir, "HEAD")
	if trunkErr == nil {
		t.Fatal("want a trunk error")
	}
	if len(classes) != 0 {
		t.Fatalf("got %d classes, want none (no fallback)", len(classes))
	}

	m := newTestModel()
	m.worktreeRoot, m.startRef = repoDir, "HEAD"
	msg := m.cmdReload()().(reloadMsg)
	if msg.trunkErr == nil {
		t.Fatal("want the reload to carry the trunk error")
	}
	next, cmd := m.handleReload(msg)
	if cmd == nil {
		t.Fatal("want a warning on the first reload")
	}
	if _, cmd = next.(Model).handleReload(msg); cmd != nil {
		t.Fatal("want no repeat warning for the same problem")
	}
}
