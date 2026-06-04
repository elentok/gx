package git_test

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestCommitFilesForRef_HEAD(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "hello.txt", "hello\n")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "add hello")

	files, err := git.CommitFilesForRef(dir, "HEAD")
	if err != nil {
		t.Fatalf("CommitFilesForRef: %v", err)
	}
	found := false
	for _, f := range files {
		if f.Path == "hello.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected hello.txt in commit files, got %+v", files)
	}
}

func TestCommitFilesForRef_EmptyRef_UsesHEAD(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "file.txt", "content\n")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "add file")

	files, err := git.CommitFilesForRef(dir, "")
	if err != nil {
		t.Fatalf("CommitFilesForRef with empty ref: %v", err)
	}
	if len(files) == 0 {
		t.Error("expected files for HEAD commit")
	}
}

func TestCommitFileDiffForRef(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "data.txt", "before\n")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "initial")
	testutil.WriteFile(t, dir, "data.txt", "after\n")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "update data")

	diff, err := git.CommitFileDiffForRef(dir, "HEAD", "data.txt", 1)
	if err != nil {
		t.Fatalf("CommitFileDiffForRef: %v", err)
	}
	if !strings.Contains(diff, "before") || !strings.Contains(diff, "after") {
		t.Errorf("unexpected diff content: %q", diff)
	}
}

func TestCommitFileDiffWithDeltaForRef(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "data.txt", "before\n")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "initial data")
	testutil.WriteFile(t, dir, "data.txt", "after\n")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "update data")

	out, err := git.CommitFileDiffWithDeltaForRef(dir, "HEAD", "data.txt", 1, 80, false)
	if err != nil {
		t.Fatalf("CommitFileDiffWithDeltaForRef: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty diff output")
	}
}

func TestCommitFilesForRef_StashIncludesTrackedAndUntracked(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "tracked.txt", "before\n")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "initial")

	testutil.WriteFile(t, dir, "tracked.txt", "after\n")
	testutil.WriteFile(t, dir, "untracked.txt", "hello\n")
	testutil.MustGitExported(t, dir, "stash", "push", "-u", "-m", "demo")

	files, err := git.CommitFilesForRef(dir, "stash@{0}")
	if err != nil {
		t.Fatalf("CommitFilesForRef stash: %v", err)
	}
	var sawTracked, sawUntracked bool
	for _, f := range files {
		if f.Path == "tracked.txt" {
			sawTracked = true
		}
		if f.Path == "untracked.txt" {
			sawUntracked = true
		}
	}
	if !sawTracked || !sawUntracked {
		t.Fatalf("expected tracked and untracked files in stash, got %+v", files)
	}
}

func TestCommitFileDiffForRef_StashTrackedFile(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "data.txt", "before\n")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "initial")
	testutil.WriteFile(t, dir, "data.txt", "after\n")
	testutil.MustGitExported(t, dir, "stash", "push", "-m", "demo")

	diff, err := git.CommitFileDiffForRef(dir, "stash@{0}", "data.txt", 1)
	if err != nil {
		t.Fatalf("CommitFileDiffForRef stash tracked: %v", err)
	}
	if !strings.Contains(diff, "before") || !strings.Contains(diff, "after") {
		t.Fatalf("unexpected stash tracked diff content: %q", diff)
	}
}

func TestCommitFileDiffForRef_StashUntrackedFile(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "base.txt", "base\n")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "initial")
	testutil.WriteFile(t, dir, "new.txt", "hello\n")
	testutil.MustGitExported(t, dir, "stash", "push", "-u", "-m", "demo")

	diff, err := git.CommitFileDiffForRef(dir, "stash@{0}", "new.txt", 1)
	if err != nil {
		t.Fatalf("CommitFileDiffForRef stash untracked: %v", err)
	}
	if !strings.Contains(diff, "new file mode") || !strings.Contains(diff, "+hello") {
		t.Fatalf("unexpected stash untracked diff content: %q", diff)
	}
}
