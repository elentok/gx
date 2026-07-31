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
}
