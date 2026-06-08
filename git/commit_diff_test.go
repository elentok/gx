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

func TestCommitImageDiffBlobs_RegularModify(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "img.png", "OLD-BYTES")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "add image")
	testutil.WriteFile(t, dir, "img.png", "NEW-BYTES")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "update image")

	old, newBytes, oldOK, newOK := git.CommitImageDiffBlobs(dir, "HEAD", git.CommitFile{Status: "M", Path: "img.png"})
	if !oldOK || !newOK {
		t.Fatalf("expected both sides present, got oldOK=%v newOK=%v", oldOK, newOK)
	}
	if string(old) != "OLD-BYTES" || string(newBytes) != "NEW-BYTES" {
		t.Fatalf("unexpected blobs: old=%q new=%q", old, newBytes)
	}
}

func TestCommitImageDiffBlobs_RegularAdd(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "seed.txt", "seed\n")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "seed")
	testutil.WriteFile(t, dir, "added.png", "ADDED")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "add png")

	old, newBytes, oldOK, newOK := git.CommitImageDiffBlobs(dir, "HEAD", git.CommitFile{Status: "A", Path: "added.png"})
	if oldOK {
		t.Fatalf("expected old side absent for an added file, got %q", old)
	}
	if !newOK || string(newBytes) != "ADDED" {
		t.Fatalf("expected new side present, got newOK=%v %q", newOK, newBytes)
	}
}

func TestCommitImageDiffBlobs_RegularDelete(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "gone.png", "GONE")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "add png")
	testutil.MustGitExported(t, dir, "rm", "gone.png")
	testutil.MustGitExported(t, dir, "commit", "-m", "remove png")

	old, _, oldOK, newOK := git.CommitImageDiffBlobs(dir, "HEAD", git.CommitFile{Status: "D", Path: "gone.png"})
	if !oldOK || string(old) != "GONE" {
		t.Fatalf("expected old side present, got oldOK=%v %q", oldOK, old)
	}
	if newOK {
		t.Fatalf("expected new side absent for a deleted file")
	}
}

func TestCommitImageDiffBlobs_Rename(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "old-name.png", "RENAMED-CONTENT")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "add png")
	testutil.MustGitExported(t, dir, "mv", "old-name.png", "new-name.png")
	testutil.MustGitExported(t, dir, "commit", "-m", "rename png")

	file := git.CommitFile{Status: "R100", Path: "new-name.png", RenameFrom: "old-name.png"}
	old, newBytes, oldOK, newOK := git.CommitImageDiffBlobs(dir, "HEAD", file)
	if !oldOK || string(old) != "RENAMED-CONTENT" {
		t.Fatalf("expected old side from rename source, got oldOK=%v %q", oldOK, old)
	}
	if !newOK || string(newBytes) != "RENAMED-CONTENT" {
		t.Fatalf("expected new side present, got newOK=%v %q", newOK, newBytes)
	}
}

func TestCommitImageDiffBlobs_StashTracked(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "img.png", "BASE")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "initial")
	testutil.WriteFile(t, dir, "img.png", "STASHED")
	testutil.MustGitExported(t, dir, "stash", "push", "-m", "demo")

	old, newBytes, oldOK, newOK := git.CommitImageDiffBlobs(dir, "stash@{0}", git.CommitFile{Status: "M", Path: "img.png"})
	if !oldOK || string(old) != "BASE" {
		t.Fatalf("expected old=BASE, got oldOK=%v %q", oldOK, old)
	}
	if !newOK || string(newBytes) != "STASHED" {
		t.Fatalf("expected new=STASHED, got newOK=%v %q", newOK, newBytes)
	}
}

func TestCommitImageDiffBlobs_StashUntracked(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "base.txt", "base\n")
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "initial")
	testutil.WriteFile(t, dir, "fresh.png", "UNTRACKED")
	testutil.MustGitExported(t, dir, "stash", "push", "-u", "-m", "demo")

	old, newBytes, oldOK, newOK := git.CommitImageDiffBlobs(dir, "stash@{0}", git.CommitFile{Status: "A", Path: "fresh.png"})
	if oldOK {
		t.Fatalf("expected old side absent for an untracked stash file, got %q", old)
	}
	if !newOK || string(newBytes) != "UNTRACKED" {
		t.Fatalf("expected new=UNTRACKED from ref^3, got newOK=%v %q", newOK, newBytes)
	}
}
