package git_test

import (
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
