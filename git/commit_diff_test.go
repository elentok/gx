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

	diff, err := git.CommitFileDiffForRef(dir, "HEAD", "data.txt")
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

	out, err := git.CommitFileDiffWithDeltaForRef(dir, "HEAD", "data.txt", 80)
	if err != nil {
		t.Fatalf("CommitFileDiffWithDeltaForRef: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty diff output")
	}
}
