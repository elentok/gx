package tickets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestPlanFoldNoopWhenNoStrays(t *testing.T) {
	actions := PlanFold("/canonical", []string{"bugs-01"}, nil)
	if len(actions) != 0 {
		t.Errorf("actions = %v, want none", actions)
	}
}

func TestPlanFoldCleanFold(t *testing.T) {
	strays := []StrayScratchDir{
		{WorktreeName: "wt-a", Path: "/wt-a/.scratch", EpicSlugs: []string{"feature-x"}},
	}
	actions := PlanFold("/canonical", []string{"bugs-01"}, strays)

	if len(actions) != 1 {
		t.Fatalf("actions = %v, want 1", actions)
	}
	got := actions[0]
	want := FoldAction{
		EpicSlug:     "feature-x",
		WorktreeName: "wt-a",
		From:         filepath.Join("/wt-a/.scratch", "feature-x"),
		To:           filepath.Join("/canonical", "feature-x"),
		Collides:     false,
	}
	if got != want {
		t.Errorf("actions[0] = %+v, want %+v", got, want)
	}
}

func TestPlanFoldSingleCollision(t *testing.T) {
	strays := []StrayScratchDir{
		{WorktreeName: "wt-a", Path: "/wt-a/.scratch", EpicSlugs: []string{"bugs-01"}},
	}
	actions := PlanFold("/canonical", []string{"bugs-01"}, strays)

	if len(actions) != 1 {
		t.Fatalf("actions = %v, want 1", actions)
	}
	if !actions[0].Collides {
		t.Errorf("actions[0].Collides = false, want true")
	}
}

func TestPlanFoldMultipleCollisions(t *testing.T) {
	strays := []StrayScratchDir{
		{WorktreeName: "wt-a", Path: "/wt-a/.scratch", EpicSlugs: []string{"bugs-01", "feature-x"}},
		{WorktreeName: "wt-b", Path: "/wt-b/.scratch", EpicSlugs: []string{"bugs-01"}},
	}
	actions := PlanFold("/canonical", []string{"bugs-01"}, strays)

	if len(actions) != 3 {
		t.Fatalf("actions = %v, want 3", actions)
	}

	// wt-a's bugs-01 collides with the canonical epic already there.
	if !actions[0].Collides {
		t.Errorf("actions[0] (wt-a bugs-01) Collides = false, want true")
	}
	// wt-a's feature-x has no collision.
	if actions[1].Collides {
		t.Errorf("actions[1] (wt-a feature-x) Collides = true, want false")
	}
	// wt-b's bugs-01 also collides, now with wt-a's claim on the same slug.
	if !actions[2].Collides {
		t.Errorf("actions[2] (wt-b bugs-01) Collides = false, want true")
	}
}

func TestApplyFoldMovesNonCollidingDirs(t *testing.T) {
	root := t.TempDir()
	strayEpic := filepath.Join(root, "stray", "feature-x")
	canonicalEpic := filepath.Join(root, "canonical", "feature-x")
	mustMkdirAll(t, strayEpic)
	mustWriteFile(t, filepath.Join(strayEpic, "map.md"), "stray")

	actions := []FoldAction{{EpicSlug: "feature-x", From: strayEpic, To: canonicalEpic, Collides: false}}
	if err := ApplyFold(actions, neverResolve(t)); err != nil {
		t.Fatalf("ApplyFold: %v", err)
	}

	assertFileContent(t, filepath.Join(canonicalEpic, "map.md"), "stray")
	assertNotExist(t, strayEpic)
}

func TestApplyFoldMergesOnResolveMerge(t *testing.T) {
	root := t.TempDir()
	strayEpic := filepath.Join(root, "stray", "bugs-01")
	canonicalEpic := filepath.Join(root, "canonical", "bugs-01")
	mustMkdirAll(t, strayEpic)
	mustMkdirAll(t, canonicalEpic)
	mustWriteFile(t, filepath.Join(strayEpic, "issue-a.md"), "from stray")
	mustWriteFile(t, filepath.Join(canonicalEpic, "issue-b.md"), "from canonical")

	actions := []FoldAction{{EpicSlug: "bugs-01", WorktreeName: "wt-a", From: strayEpic, To: canonicalEpic, Collides: true}}
	resolve := func(FoldAction) (CollisionResolution, error) { return ResolveMerge, nil }
	if err := ApplyFold(actions, resolve); err != nil {
		t.Fatalf("ApplyFold: %v", err)
	}

	assertFileContent(t, filepath.Join(canonicalEpic, "issue-a.md"), "from stray")
	assertFileContent(t, filepath.Join(canonicalEpic, "issue-b.md"), "from canonical")
	assertNotExist(t, strayEpic)
}

func TestApplyFoldMergeLeavesConflictingEntriesAndErrors(t *testing.T) {
	root := t.TempDir()
	strayEpic := filepath.Join(root, "stray", "bugs-01")
	canonicalEpic := filepath.Join(root, "canonical", "bugs-01")
	mustMkdirAll(t, strayEpic)
	mustMkdirAll(t, canonicalEpic)
	mustWriteFile(t, filepath.Join(strayEpic, "issue-a.md"), "stray version")
	mustWriteFile(t, filepath.Join(canonicalEpic, "issue-a.md"), "canonical version")

	actions := []FoldAction{{EpicSlug: "bugs-01", WorktreeName: "wt-a", From: strayEpic, To: canonicalEpic, Collides: true}}
	resolve := func(FoldAction) (CollisionResolution, error) { return ResolveMerge, nil }
	if err := ApplyFold(actions, resolve); err == nil {
		t.Fatal("ApplyFold: want error on conflicting entry, got nil")
	}

	// Nothing overwritten or lost: both versions still exist, one on each side.
	assertFileContent(t, filepath.Join(canonicalEpic, "issue-a.md"), "canonical version")
	assertFileContent(t, filepath.Join(strayEpic, "issue-a.md"), "stray version")
}

func TestApplyFoldRenamesOnResolveAutoRename(t *testing.T) {
	root := t.TempDir()
	strayEpic := filepath.Join(root, "stray", "bugs-01")
	canonicalEpic := filepath.Join(root, "canonical", "bugs-01")
	mustMkdirAll(t, strayEpic)
	mustMkdirAll(t, canonicalEpic)
	mustWriteFile(t, filepath.Join(strayEpic, "issue-a.md"), "stray")
	mustWriteFile(t, filepath.Join(canonicalEpic, "issue-b.md"), "canonical")

	actions := []FoldAction{{EpicSlug: "bugs-01", WorktreeName: "wt-a", From: strayEpic, To: canonicalEpic, Collides: true}}
	resolve := func(FoldAction) (CollisionResolution, error) { return ResolveAutoRename, nil }
	if err := ApplyFold(actions, resolve); err != nil {
		t.Fatalf("ApplyFold: %v", err)
	}

	renamed := canonicalEpic + "-wt-a"
	assertFileContent(t, filepath.Join(renamed, "issue-a.md"), "stray")
	assertFileContent(t, filepath.Join(canonicalEpic, "issue-b.md"), "canonical")
	assertNotExist(t, strayEpic)
}

func TestGatherStrayScratchDirsSkipsEmptyWorktreesAndCanonicalRoot(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	mustMkdirAll(t, filepath.Join(repoDir, "feature-a", ".scratch", "bugs-01"))
	// feature-b has no .scratch at all - must be skipped, not errored on.
	mustMkdirAll(t, repo.ScratchRoot()) // canonical root itself must never appear as a stray

	strays, err := GatherStrayScratchDirs(*repo)
	if err != nil {
		t.Fatalf("GatherStrayScratchDirs: %v", err)
	}

	if len(strays) != 1 {
		t.Fatalf("strays = %+v, want 1", strays)
	}
	if strays[0].WorktreeName != "feature-a" {
		t.Errorf("WorktreeName = %q, want feature-a", strays[0].WorktreeName)
	}
	if len(strays[0].EpicSlugs) != 1 || strays[0].EpicSlugs[0] != "bugs-01" {
		t.Errorf("EpicSlugs = %v, want [bugs-01]", strays[0].EpicSlugs)
	}
}

func TestFoldStrayScratchDirsNoStrayIsNoop(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	if err := FoldStrayScratchDirs(*repo, neverResolve(t)); err != nil {
		t.Fatalf("FoldStrayScratchDirs: %v", err)
	}
	assertNotExist(t, repo.ScratchRoot())
}

func TestFoldStrayScratchDirsCleanFold(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	strayEpic := filepath.Join(repoDir, "feature-a", ".scratch", "bugs-01")
	mustMkdirAll(t, strayEpic)
	mustWriteFile(t, filepath.Join(strayEpic, "map.md"), "stray content")

	if err := FoldStrayScratchDirs(*repo, neverResolve(t)); err != nil {
		t.Fatalf("FoldStrayScratchDirs: %v", err)
	}

	assertFileContent(t, filepath.Join(repo.ScratchRoot(), "bugs-01", "map.md"), "stray content")
	assertNotExist(t, strayEpic)
}

func TestFoldStrayScratchDirsCollisionPromptsAndResolves(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	strayEpic := filepath.Join(repoDir, "feature-a", ".scratch", "bugs-01")
	mustMkdirAll(t, strayEpic)
	mustWriteFile(t, filepath.Join(strayEpic, "issue-a.md"), "stray")

	canonicalEpic := filepath.Join(repo.ScratchRoot(), "bugs-01")
	mustMkdirAll(t, canonicalEpic)
	mustWriteFile(t, filepath.Join(canonicalEpic, "issue-b.md"), "canonical")

	prompted := false
	resolve := func(action FoldAction) (CollisionResolution, error) {
		prompted = true
		if action.EpicSlug != "bugs-01" || action.WorktreeName != "feature-a" {
			t.Errorf("unexpected action %+v", action)
		}
		return ResolveAutoRename, nil
	}

	if err := FoldStrayScratchDirs(*repo, resolve); err != nil {
		t.Fatalf("FoldStrayScratchDirs: %v", err)
	}
	if !prompted {
		t.Error("resolve was never called for the colliding epic")
	}

	assertFileContent(t, filepath.Join(canonicalEpic+"-feature-a", "issue-a.md"), "stray")
	assertFileContent(t, filepath.Join(canonicalEpic, "issue-b.md"), "canonical")
	assertNotExist(t, strayEpic)
}

func TestFoldStrayScratchDirsMergeRecursiveNoRealConflict(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	strayEpic := filepath.Join(repoDir, "feature-a", ".scratch", "epic1")
	mustMkdirAll(t, filepath.Join(strayEpic, "issues"))
	mustWriteFile(t, filepath.Join(strayEpic, "issues", "02-ticket2.md"), "stray ticket")

	canonicalEpic := filepath.Join(repo.ScratchRoot(), "epic1")
	mustMkdirAll(t, filepath.Join(canonicalEpic, "issues"))
	mustWriteFile(t, filepath.Join(canonicalEpic, "issues", "01-ticket.md"), "canonical ticket")

	resolve := func(FoldAction) (CollisionResolution, error) { return ResolveMerge, nil }
	if err := FoldStrayScratchDirs(*repo, resolve); err != nil {
		t.Fatalf("FoldStrayScratchDirs: %v", err)
	}

	assertFileContent(t, filepath.Join(canonicalEpic, "issues", "01-ticket.md"), "canonical ticket")
	assertFileContent(t, filepath.Join(canonicalEpic, "issues", "02-ticket2.md"), "stray ticket")
	assertNotExist(t, strayEpic)
}

func TestFoldStrayScratchDirsMergeRealConflictLeavesBothTreesUntouched(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	strayEpic := filepath.Join(repoDir, "feature-a", ".scratch", "epic1")
	mustMkdirAll(t, filepath.Join(strayEpic, "issues"))
	mustWriteFile(t, filepath.Join(strayEpic, "issues", "01-ticket.md"), "stray version")
	mustWriteFile(t, filepath.Join(strayEpic, "issues", "02-ticket2.md"), "stray only")

	canonicalEpic := filepath.Join(repo.ScratchRoot(), "epic1")
	mustMkdirAll(t, filepath.Join(canonicalEpic, "issues"))
	mustWriteFile(t, filepath.Join(canonicalEpic, "issues", "01-ticket.md"), "canonical version")

	resolve := func(FoldAction) (CollisionResolution, error) { return ResolveMerge, nil }
	if err := FoldStrayScratchDirs(*repo, resolve); err == nil {
		t.Fatal("FoldStrayScratchDirs: want error on real conflict, got nil")
	}

	// All-or-nothing: even the non-colliding ticket must stay put on both sides.
	assertFileContent(t, filepath.Join(strayEpic, "issues", "01-ticket.md"), "stray version")
	assertFileContent(t, filepath.Join(strayEpic, "issues", "02-ticket2.md"), "stray only")
	assertFileContent(t, filepath.Join(canonicalEpic, "issues", "01-ticket.md"), "canonical version")
	assertNotExist(t, filepath.Join(canonicalEpic, "issues", "02-ticket2.md"))
}

func neverResolve(t *testing.T) ResolveCollision {
	t.Helper()
	return func(action FoldAction) (CollisionResolution, error) {
		t.Fatalf("resolve called unexpectedly for %+v", action)
		return 0, nil
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Errorf("%s content = %q, want %q", path, data, want)
	}
}

func assertNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s still exists, want removed", path)
	}
}
