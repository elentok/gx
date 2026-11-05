package git_test

import (
	"slices"
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

func TestCommitSubjects_ListsRangeOldestFirst(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	if subjects, err := git.CommitSubjects(dir, base, "HEAD"); err != nil || subjects != nil {
		t.Fatalf("CommitSubjects(empty range) = %v, %v; want nil, nil", subjects, err)
	}

	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	testutil.WriteFile(t, dir, "b.txt", "b\n")
	testutil.CommitAll(t, dir, "add b")

	subjects, err := git.CommitSubjects(dir, base, "HEAD")
	if err != nil {
		t.Fatalf("CommitSubjects: %v", err)
	}
	if want := []string{"add a", "add b"}; !slices.Equal(subjects, want) {
		t.Errorf("CommitSubjects() = %q, want %q", subjects, want)
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

func TestAttachDetachedHead_MovesBranchToDetachedCommit(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	// An agent's `git checkout -q HEAD~0` detaches HEAD; its next commit lands
	// there and the iteration branch stays behind at its base.
	testutil.MustGitExported(t, dir, "checkout", "-q", "-b", "iter")
	testutil.MustGitExported(t, dir, "checkout", "-q", "HEAD~0")
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	head, _ := git.RevParse(dir, "HEAD")

	moved, err := git.AttachDetachedHead(dir, "iter")
	if err != nil {
		t.Fatalf("AttachDetachedHead: %v", err)
	}
	if !moved {
		t.Errorf("AttachDetachedHead() moved = false, want true")
	}
	if got, _ := git.RevParse(dir, "iter"); got != head {
		t.Errorf("iter = %q, want the detached commit %q", got, head)
	}
	if got, _ := git.CurrentBranch(dir); got != "iter" {
		t.Errorf("CurrentBranch() = %q, want iter", got)
	}
}

func TestAttachDetachedHead_LeavesAttachedHeadAlone(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.MustGitExported(t, dir, "checkout", "-q", "-b", "iter")

	moved, err := git.AttachDetachedHead(dir, "iter")
	if err != nil || moved {
		t.Errorf("AttachDetachedHead() = %v, %v; want false, nil", moved, err)
	}
}

func TestAttachDetachedHead_LeavesUnrelatedDetachedHeadAlone(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	base, _ := git.RevParse(dir, "HEAD")
	testutil.MustGitExported(t, dir, "checkout", "-q", "-b", "iter")
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	iter, _ := git.RevParse(dir, "iter")

	// HEAD detached at a commit the branch does not lead to: moving the
	// branch there would drop its own commit.
	testutil.MustGitExported(t, dir, "checkout", "-q", base)

	moved, err := git.AttachDetachedHead(dir, "iter")
	if err != nil || moved {
		t.Errorf("AttachDetachedHead() = %v, %v; want false, nil", moved, err)
	}
	if got, _ := git.RevParse(dir, "iter"); got != iter {
		t.Errorf("iter = %q, want it unchanged at %q", got, iter)
	}
}
