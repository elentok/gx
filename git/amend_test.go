package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestIsHEAD_True(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	hash, _, err := run(dir, []string{"rev-parse", "HEAD"})
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	ok, err := IsHEAD(dir, hash)
	if err != nil {
		t.Fatalf("IsHEAD: %v", err)
	}
	if !ok {
		t.Error("expected IsHEAD=true for HEAD hash")
	}
}

func TestIsHEAD_False(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "x.txt", "x")
	testutil.CommitAll(t, dir, "second")
	// get first commit hash
	log, _, err := run(dir, []string{"log", "--format=%H", "-n", "2"})
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected 2 commits, got %d", len(lines))
	}
	firstHash := lines[1]
	ok, err := IsHEAD(dir, firstHash)
	if err != nil {
		t.Fatalf("IsHEAD: %v", err)
	}
	if ok {
		t.Error("expected IsHEAD=false for non-HEAD hash")
	}
}

func TestIsCommitPushed_NotPushed(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	hash, _, err := run(dir, []string{"rev-parse", "HEAD"})
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	pushed, err := IsCommitPushed(dir, hash)
	if err != nil {
		t.Fatalf("IsCommitPushed: %v", err)
	}
	if pushed {
		t.Error("expected not pushed in repo with no remote")
	}
}

func TestStagedFiles_Empty(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	files, err := StagedFiles(dir)
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected no staged files, got %v", files)
	}
}

func TestStagedFiles_WithStagedFile(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "staged.txt", "hello")
	if _, _, err := run(dir, []string{"add", "staged.txt"}); err != nil {
		t.Fatalf("git add: %v", err)
	}
	files, err := StagedFiles(dir)
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "staged.txt" {
		t.Errorf("expected [staged.txt], got %v", files)
	}
}

func TestHasUnstagedChanges_False(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	has, err := HasUnstagedChanges(dir)
	if err != nil {
		t.Fatalf("HasUnstagedChanges: %v", err)
	}
	if has {
		t.Error("expected no unstaged changes in clean repo")
	}
}

func TestHasUnstagedChanges_True(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	// modify a tracked file without staging it
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	has, err := HasUnstagedChanges(dir)
	if err != nil {
		t.Fatalf("HasUnstagedChanges: %v", err)
	}
	if !has {
		t.Error("expected unstaged changes after modifying tracked file")
	}
}

func TestAmendHead(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "extra.txt", "added\n")
	testutil.MustGitExported(t, dir, "add", "extra.txt")
	out, err := AmendHead(dir)
	if err != nil {
		t.Fatalf("AmendHead: %v\n%s", err, out)
	}
}

func TestCommitFixup(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	hash, _, err := run(dir, []string{"rev-parse", "HEAD"})
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	hash = strings.TrimSpace(hash)

	testutil.WriteFile(t, dir, "fix.txt", "fix\n")
	testutil.MustGitExported(t, dir, "add", "fix.txt")
	out, err := CommitFixup(dir, hash)
	if err != nil {
		t.Fatalf("CommitFixup: %v\n%s", err, out)
	}
}

func TestStashPushAuto_WithChanges(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := StashPushAuto(dir)
	if err != nil {
		t.Fatalf("StashPushAuto: %v\n%s", err, out)
	}
}
