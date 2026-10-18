package tickets

import (
	"testing"

	"github.com/elentok/gx/testutil"
)

// countGitReads swaps the cached git reads for counting ones until the test
// ends. Not parallel-safe (package vars).
func countGitReads(t *testing.T) *int {
	t.Helper()
	calls := 0
	origDefault, origValid, origExists := gitRemoteDefaultBranch, gitValidBranchName, gitRefExists
	gitRemoteDefaultBranch = func(repo string) string { calls++; return origDefault(repo) }
	gitValidBranchName = func(name string) bool { calls++; return origValid(name) }
	gitRefExists = func(repo, ref string) bool { calls++; return origExists(repo, ref) }
	t.Cleanup(func() { gitRemoteDefaultBranch, gitValidBranchName, gitRefExists = origDefault, origValid, origExists })
	return &calls
}

func mustTrunk(t *testing.T, projectDir, repo, want string) {
	t.Helper()
	got, err := Trunk(projectDir, repo)
	if err != nil || got != want {
		t.Fatalf("Trunk() = %q, %v; want %q", got, err, want)
	}
}

func TestTrunk_RepeatResolveRunsNoGit(t *testing.T) {
	for name, newRepo := range map[string]func(*testing.T) string{
		"work tree": testutil.TempRepo,
		"bare":      testutil.TempBareRepo,
	} {
		t.Run(name, func(t *testing.T) {
			_, projectDir := trunkFixture(t, "")
			repo := newRepo(t)
			testutil.WriteFile(t, projectDir, "project.json", registered(repo, ""))
			calls := countGitReads(t)

			mustTrunk(t, projectDir, repo, "main")
			first := *calls
			if first == 0 {
				t.Fatal("first resolve ran no git reads")
			}
			mustTrunk(t, projectDir, repo, "main")
			if *calls != first {
				t.Errorf("second resolve ran %d git reads, want 0", *calls-first)
			}
		})
	}
}

func TestTrunk_SeesABranchCreatedAfterACachedMiss(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, `,"trunk":"develop"`))
	if _, err := Trunk(projectDir, repo); err == nil {
		t.Fatal("Trunk() before the branch exists: want an error")
	}

	testutil.MustGitExported(t, repo, "branch", "develop")
	mustTrunk(t, projectDir, repo, "develop")
}

func TestTrunk_SeesABranchDeletedAfterACachedHit(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.MustGitExported(t, repo, "branch", "develop")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, `,"trunk":"develop"`))
	mustTrunk(t, projectDir, repo, "develop")

	testutil.MustGitExported(t, repo, "branch", "-D", "develop")
	if _, err := Trunk(projectDir, repo); err == nil {
		t.Fatal("Trunk() after the branch is deleted: want an error")
	}
}

func TestTrunk_SeesAPackedBranchDeleted(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.MustGitExported(t, repo, "branch", "develop")
	testutil.MustGitExported(t, repo, "pack-refs", "--all")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, `,"trunk":"develop"`))
	mustTrunk(t, projectDir, repo, "develop")

	testutil.MustGitExported(t, repo, "branch", "-D", "develop")
	if _, err := Trunk(projectDir, repo); err == nil {
		t.Fatal("Trunk() after the packed branch is deleted: want an error")
	}
}

func TestTrunk_DetectedDefaultFollowsOriginHEAD(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.MustGitExported(t, repo, "branch", "develop")
	testutil.MustGitExported(t, repo, "update-ref", "refs/remotes/origin/develop", "HEAD")
	testutil.MustGitExported(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	testutil.MustGitExported(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, ""))
	mustTrunk(t, projectDir, repo, "develop")

	testutil.MustGitExported(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	mustTrunk(t, projectDir, repo, "main")
}

func TestTrunk_ProjectFileEditAppliesAtOnce(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.MustGitExported(t, repo, "branch", "develop")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, ""))
	mustTrunk(t, projectDir, repo, "main")

	testutil.WriteFile(t, projectDir, "project.json", registered(repo, `,"trunk":"develop"`))
	mustTrunk(t, projectDir, repo, "develop")
}

// A layout the stamp can't read (here a linked worktree's .git file) is never
// cached: every resolve asks git.
func TestTrunk_UnknownLayoutIsNotCached(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	wt := t.TempDir() + "/wt"
	testutil.MustGitExported(t, repo, "worktree", "add", "-b", "wt", wt)
	testutil.WriteFile(t, projectDir, "project.json", registered(wt, ""))
	calls := countGitReads(t)

	mustTrunk(t, projectDir, wt, "main")
	first := *calls
	if first == 0 {
		t.Fatal("first resolve ran no git reads")
	}
	mustTrunk(t, projectDir, wt, "main")
	if *calls != 2*first {
		t.Errorf("second resolve ran %d git reads, want %d", *calls-first, first)
	}
}
