package git_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestRevParse(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	hash, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	if len(hash) != 40 {
		t.Errorf("RevParse() = %q, want a 40-char commit hash", hash)
	}
}

func TestMergeBase_FindsCommonAncestorEvenAfterOtherRefAdvances(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "iter", base)
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")

	// feature advances past base after iter branched off it, the way the
	// shared feature branch does while other tickets land during a crashed
	// invocation's downtime.
	testutil.MustGitExported(t, dir, "checkout", "-b", "feature", base)
	testutil.WriteFile(t, dir, "b.txt", "b\n")
	testutil.CommitAll(t, dir, "add b")

	got, err := git.MergeBase(dir, "iter", "feature")
	if err != nil {
		t.Fatalf("MergeBase: %v", err)
	}
	if got != base {
		t.Errorf("MergeBase() = %q, want the original branch point %q", got, base)
	}
}

func TestCherryPickRange_AppliesCommitsInOrder(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "iter")
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	testutil.WriteFile(t, dir, "b.txt", "b\n")
	testutil.CommitAll(t, dir, "add b")
	tip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "feature", base)

	if err := git.CherryPickRange(dir, base, tip); err != nil {
		t.Fatalf("CherryPickRange: %v", err)
	}

	out, err := exec.Command("git", "-C", dir, "log", "--format=%s", "-2").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	log := string(out)
	if !strings.Contains(log, "add a") || !strings.Contains(log, "add b") {
		t.Errorf("log after cherry-pick = %q, want both commits", log)
	}
}

func TestCherryPickRange_ConflictReturnsError(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepoWithConflictSetup(t)

	base, err := git.RevParse(dir, "HEAD~1")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	tip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "feature", base)
	testutil.WriteFile(t, dir, "a.txt", "line from feature\n")
	testutil.CommitAll(t, dir, "feature change")

	if err := git.CherryPickRange(dir, base, tip); err == nil {
		t.Fatal("CherryPickRange() error = nil, want conflict error")
	}

	inProgress, err := git.CherryPickInProgress(dir)
	if err != nil {
		t.Fatalf("CherryPickInProgress: %v", err)
	}
	if !inProgress {
		t.Error("CherryPickInProgress() = false, want true while mid-conflict")
	}
}

func TestCommitsAhead_CountsCommitsInRange(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	testutil.WriteFile(t, dir, "b.txt", "b\n")
	testutil.CommitAll(t, dir, "add b")

	ahead, err := git.CommitsAhead(dir, base, "HEAD")
	if err != nil {
		t.Fatalf("CommitsAhead: %v", err)
	}
	if ahead != 2 {
		t.Errorf("CommitsAhead() = %d, want 2", ahead)
	}
}

func TestCommitsAhead_ZeroWhenNoNewCommits(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	ahead, err := git.CommitsAhead(dir, base, "HEAD")
	if err != nil {
		t.Fatalf("CommitsAhead: %v", err)
	}
	if ahead != 0 {
		t.Errorf("CommitsAhead() = %d, want 0", ahead)
	}
}

func TestIsAncestor_True(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")

	ok, err := git.IsAncestor(dir, base, "HEAD")
	if err != nil {
		t.Fatalf("IsAncestor: %v", err)
	}
	if !ok {
		t.Error("IsAncestor() = false, want true for a real ancestor")
	}
}

func TestIsAncestor_False(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	testutil.MustGitExported(t, dir, "checkout", "-b", "other", base)
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	other, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "feature", base)

	ok, err := git.IsAncestor(dir, other, "feature")
	if err != nil {
		t.Fatalf("IsAncestor: %v", err)
	}
	if ok {
		t.Error("IsAncestor() = true, want false when the commit was never merged in")
	}
}

func TestCherryPickInProgress_FalseOutsideCherryPick(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	inProgress, err := git.CherryPickInProgress(dir)
	if err != nil {
		t.Fatalf("CherryPickInProgress: %v", err)
	}
	if inProgress {
		t.Error("CherryPickInProgress() = true, want false outside a cherry-pick")
	}
}
