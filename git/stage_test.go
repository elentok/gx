package git

import (
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestListStageFiles(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	testutil.WriteFile(t, repo, "README.md", "first\n")
	testutil.MustGitExported(t, repo, "add", "README.md")
	testutil.WriteFile(t, repo, "README.md", "second\n")
	testutil.WriteFile(t, repo, "new.txt", "hello\n")

	files, err := ListStageFiles(repo)
	if err != nil {
		t.Fatalf("ListStageFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	byPath := map[string]StageFileStatus{}
	for _, f := range files {
		byPath[f.Path] = f
	}

	readme, ok := byPath["README.md"]
	if !ok {
		t.Fatalf("missing README.md in status: %+v", files)
	}
	if !readme.HasStagedChanges() || !readme.HasUnstagedChanges() {
		t.Fatalf("README.md expected staged+unstaged, got %s", readme.XY())
	}

	untracked, ok := byPath["new.txt"]
	if !ok {
		t.Fatalf("missing new.txt in status: %+v", files)
	}
	if !untracked.IsUntracked() {
		t.Fatalf("new.txt expected untracked, got %s", untracked.XY())
	}
}

func TestDiffUntrackedPath(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "new.txt", "hello\n")

	raw, err := DiffUntrackedPath(repo, "new.txt", false, false, 0, 1)
	if err != nil {
		t.Fatalf("DiffUntrackedPath raw: %v", err)
	}
	if !strings.Contains(raw, "+++ ") || !strings.Contains(raw, "/dev/null") {
		t.Fatalf("unexpected untracked diff:\n%s", raw)
	}

	color, err := DiffUntrackedPath(repo, "new.txt", true, false, 0, 1)
	if err != nil {
		t.Fatalf("DiffUntrackedPath color: %v", err)
	}
	if strings.TrimSpace(color) == "" {
		t.Fatalf("expected non-empty colored untracked diff")
	}
}

func TestListStageFiles_UntrackedDirectoryListsFiles(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/newdir")
	testutil.WriteFile(t, repo, "newdir/a.txt", "a\n")
	testutil.WriteFile(t, repo, "newdir/b.txt", "b\n")

	files, err := ListStageFiles(repo)
	if err != nil {
		t.Fatalf("ListStageFiles: %v", err)
	}

	seen := map[string]bool{}
	for _, f := range files {
		seen[f.Path] = true
	}
	if !seen["newdir/a.txt"] || !seen["newdir/b.txt"] {
		t.Fatalf("expected untracked files in nested dir, got %#v", files)
	}
	if seen["newdir/"] {
		t.Fatalf("unexpected aggregated dir entry: %#v", files)
	}
}

func TestDiffPath_ContextLinesAffectHunkGrouping(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\n")
	testutil.MustGitExported(t, repo, "add", "README.md")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	testutil.WriteFile(t, repo, "README.md", "L1\nl2\nl3\nl4\nl5\nl6\nl7\nL8\n")

	compact, err := DiffPath(repo, "README.md", false, 0)
	if err != nil {
		t.Fatalf("DiffPath compact: %v", err)
	}
	wider, err := DiffPath(repo, "README.md", false, 5)
	if err != nil {
		t.Fatalf("DiffPath wider: %v", err)
	}

	compactHunks := strings.Count(compact, "@@ ")
	widerHunks := strings.Count(wider, "@@ ")
	if compactHunks <= widerHunks {
		t.Fatalf("expected fewer hunks with larger context, compact=%d wider=%d", compactHunks, widerHunks)
	}
}

func TestStageFileStatusHelpers(t *testing.T) {
	t.Parallel()

	modified := StageFileStatus{IndexStatus: 'M', WorktreeCode: ' ', Path: "foo.go"}
	if modified.XY() != "M " {
		t.Errorf("XY = %q, want 'M '", modified.XY())
	}
	if modified.IsRenamed() {
		t.Error("modified should not be renamed")
	}

	renamed := StageFileStatus{IndexStatus: 'R', WorktreeCode: ' ', Path: "new.go", RenameFrom: "old.go"}
	if !renamed.IsRenamed() {
		t.Error("expected renamed=true for IndexStatus=R")
	}
}

func TestWorkTreeRoot_InsideRepo(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	root, err := WorktreeRoot(repo)
	if err != nil {
		t.Fatalf("WorktreeRoot: %v", err)
	}
	if root == "" {
		t.Error("expected non-empty root")
	}
}

func TestUnstagePath_RemovesStagedChange(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "staged.txt", "content\n")
	testutil.MustGitExported(t, repo, "add", "staged.txt")

	if err := UnstagePath(repo, "staged.txt"); err != nil {
		t.Fatalf("UnstagePath: %v", err)
	}

	files, err := ListStageFiles(repo)
	if err != nil {
		t.Fatalf("ListStageFiles: %v", err)
	}
	for _, f := range files {
		if f.Path == "staged.txt" && f.HasStagedChanges() {
			t.Fatal("expected staged.txt to be unstaged after UnstagePath")
		}
	}
}

func TestStagePath_StagesFile(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "newfile.txt", "hello\n")

	if err := StagePath(repo, "newfile.txt"); err != nil {
		t.Fatalf("StagePath: %v", err)
	}

	staged, err := StagedFiles(repo)
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	found := false
	for _, f := range staged {
		if f == "newfile.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected newfile.txt in staged files, got %v", staged)
	}
}

func TestListStageFiles_RenameTracksSourceAndDestination(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "old.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "old.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "add old")
	testutil.MustGitExported(t, repo, "mv", "old.txt", "new.txt")

	files, err := ListStageFiles(repo)
	if err != nil {
		t.Fatalf("ListStageFiles: %v", err)
	}

	var renamed *StageFileStatus
	for i := range files {
		if files[i].Path == "new.txt" {
			renamed = &files[i]
			break
		}
	}
	if renamed == nil {
		t.Fatalf("expected renamed destination entry new.txt, got %+v", files)
	}
	if renamed.RenameFrom != "old.txt" {
		t.Fatalf("expected rename source old.txt, got %q", renamed.RenameFrom)
	}
}
