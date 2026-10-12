package git_test

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestMergeFastForward_Clean(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	testutil.MustGitExported(t, dir, "checkout", "-b", "feature")
	testutil.WriteFile(t, dir, "feature.txt", "hi\n")
	testutil.CommitAll(t, dir, "add feature.txt")
	testutil.MustGitExported(t, dir, "checkout", "main")

	ok, err := git.MergeFastForward(dir, "feature")
	if err != nil {
		t.Fatalf("MergeFastForward: %v", err)
	}
	if !ok {
		t.Error("MergeFastForward() ok = false, want true for a clean fast-forward")
	}
}

func TestMergeFastForward_Diverged(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	testutil.MustGitExported(t, dir, "checkout", "-b", "feature")
	testutil.WriteFile(t, dir, "feature.txt", "hi\n")
	testutil.CommitAll(t, dir, "add feature.txt")
	testutil.MustGitExported(t, dir, "checkout", "main")
	testutil.WriteFile(t, dir, "main-only.txt", "diverge\n")
	testutil.CommitAll(t, dir, "add main-only.txt")

	ok, err := git.MergeFastForward(dir, "feature")
	if err != nil {
		t.Fatalf("MergeFastForward: %v", err)
	}
	if ok {
		t.Error("MergeFastForward() ok = true, want false for diverged branches")
	}
}

// TestMergeFastForward_DivergedNonEnglishLocale guards against detecting
// needs_rebase by matching git's (locale-dependent) stderr text: under a
// non-English LC_ALL/LANG git would emit a translated message, and a
// stderr-matching implementation would misreport this as a hard error.
func TestMergeFastForward_DivergedNonEnglishLocale(t *testing.T) {
	dir := testutil.TempRepo(t)

	testutil.MustGitExported(t, dir, "checkout", "-b", "feature")
	testutil.WriteFile(t, dir, "feature.txt", "hi\n")
	testutil.CommitAll(t, dir, "add feature.txt")
	testutil.MustGitExported(t, dir, "checkout", "main")
	testutil.WriteFile(t, dir, "main-only.txt", "diverge\n")
	testutil.CommitAll(t, dir, "add main-only.txt")

	t.Setenv("LC_ALL", "de_DE.UTF-8")
	t.Setenv("LANG", "de_DE.UTF-8")

	ok, err := git.MergeFastForward(dir, "feature")
	if err != nil {
		t.Fatalf("MergeFastForward: %v", err)
	}
	if ok {
		t.Error("MergeFastForward() ok = true, want false (needs_rebase) under a non-English locale")
	}
}

// featureAhead commits on a new feature branch off the current HEAD of dir,
// then checks start back out.
func featureAhead(t *testing.T, dir, start string) {
	t.Helper()
	testutil.MustGitExported(t, dir, "checkout", "-b", "feature")
	testutil.WriteFile(t, dir, "feature.txt", "hi\n")
	testutil.CommitAll(t, dir, "add feature.txt")
	testutil.MustGitExported(t, dir, "checkout", start)
}

func mustRevParse(t *testing.T, dir, ref string) string {
	t.Helper()
	sha, err := git.RevParse(dir, ref)
	if err != nil {
		t.Fatalf("RevParse(%s): %v", ref, err)
	}
	return sha
}

func TestMergeBranchInto_MissingTargetRefused(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	featureAhead(t, dir, "main")
	mainBefore := mustRevParse(t, dir, "main")

	_, err := git.MergeBranchInto(git.Repo{Root: dir}, nil, "feature", "develop")
	if err == nil || !strings.Contains(err.Error(), `"develop" does not exist`) {
		t.Fatalf("MergeBranchInto() err = %v, want a missing-target error", err)
	}
	if got := mustRevParse(t, dir, "main"); got != mainBefore {
		t.Error("main moved on a refused merge")
	}
}

func TestMergeBranchInto_TargetCheckedOutInWorktree(t *testing.T) {
	t.Parallel()
	repoDir, wtDir := testutil.TempRepoWithLinkedWorktree(t, "develop")
	featureAhead(t, repoDir, "main")
	worktrees := []git.Worktree{{Path: repoDir, Branch: "main"}, {Path: wtDir, Branch: "develop"}}
	mainBefore := mustRevParse(t, repoDir, "main")

	out, err := git.MergeBranchInto(git.Repo{Root: repoDir}, worktrees, "feature", "develop")
	if err != nil || !out.Merged {
		t.Fatalf("MergeBranchInto() = %+v, %v; want merged", out, err)
	}
	if got := mustRevParse(t, wtDir, "HEAD"); got != mustRevParse(t, repoDir, "feature") {
		t.Error("develop worktree HEAD did not advance to feature")
	}
	if got := mustRevParse(t, repoDir, "main"); got != mainBefore {
		t.Error("main moved")
	}
}

func TestMergeBranchInto_TargetIsRootHead(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	featureAhead(t, dir, "main")

	out, err := git.MergeBranchInto(git.Repo{Root: dir}, nil, "feature", "main")
	if err != nil || !out.Merged {
		t.Fatalf("MergeBranchInto() = %+v, %v; want merged", out, err)
	}
	if got := mustRevParse(t, dir, "HEAD"); got != mustRevParse(t, dir, "feature") {
		t.Error("root HEAD did not advance to feature")
	}
	testutil.MustGitExported(t, dir, "diff", "--quiet", "HEAD")
}

func TestMergeBranchInto_TargetNotCheckedOutAdvancesRefOnly(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.MustGitExported(t, dir, "branch", "develop")
	featureAhead(t, dir, "main")
	mainBefore := mustRevParse(t, dir, "main")

	out, err := git.MergeBranchInto(git.Repo{Root: dir}, nil, "feature", "develop")
	if err != nil || !out.Merged {
		t.Fatalf("MergeBranchInto() = %+v, %v; want merged", out, err)
	}
	if got := mustRevParse(t, dir, "develop"); got != mustRevParse(t, dir, "feature") {
		t.Error("develop did not advance to feature")
	}
	if got := mustRevParse(t, dir, "main"); got != mainBefore {
		t.Error("main moved, want it left alone")
	}
	if branch, _ := git.CurrentBranch(dir); branch != "main" {
		t.Errorf("root checkout = %q, want main", branch)
	}
}

func TestMergeBranchInto_DivergedTargetNeedsRebase(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	featureAhead(t, dir, "main")
	testutil.MustGitExported(t, dir, "checkout", "-b", "develop")
	testutil.WriteFile(t, dir, "develop-only.txt", "diverge\n")
	testutil.CommitAll(t, dir, "add develop-only.txt")
	testutil.MustGitExported(t, dir, "checkout", "main")
	developBefore := mustRevParse(t, dir, "develop")

	out, err := git.MergeBranchInto(git.Repo{Root: dir}, nil, "feature", "develop")
	if err != nil {
		t.Fatalf("MergeBranchInto: %v", err)
	}
	if out.Merged || out.Branch != "feature" || out.Target != "develop" {
		t.Errorf("MergeBranchInto() = %+v, want needs-rebase for feature into develop", out)
	}
	if got := mustRevParse(t, dir, "develop"); got != developBefore {
		t.Error("develop moved on a needs-rebase outcome")
	}
}

// TestMergeFastForward_HardFailure exercises a real merge failure distinct
// from a plain non-fast-forward refusal: an untracked file collides with one
// the merge would introduce.
func TestMergeFastForward_HardFailure(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	testutil.MustGitExported(t, dir, "checkout", "-b", "feature")
	testutil.WriteFile(t, dir, "conflict.txt", "from feature\n")
	testutil.CommitAll(t, dir, "add conflict.txt")
	testutil.MustGitExported(t, dir, "checkout", "main")
	testutil.WriteFile(t, dir, "conflict.txt", "untracked local content\n")

	ok, err := git.MergeFastForward(dir, "feature")
	if ok {
		t.Error("MergeFastForward() ok = true, want false for a real failure")
	}
	if err == nil {
		t.Error("MergeFastForward() err = nil, want non-nil for an untracked-file collision")
	}
}
