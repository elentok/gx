package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestStash_CleanRepo(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	// Stash on clean repo produces "No local changes to save"
	out, err := Stash(dir)
	if err != nil {
		t.Fatalf("Stash on clean repo: %v\n%s", err, out)
	}
}

func TestStash_WithChanges(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("stash me"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := Stash(dir)
	if err != nil {
		t.Fatalf("Stash: %v\n%s", err, out)
	}
	// restore should succeed
	popOut, popErr := StashPop(dir)
	if popErr != nil {
		t.Fatalf("StashPop: %v\n%s", popErr, popOut)
	}
}

func TestStashPush_AllWithName(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	writeFile(t, dir, "README.md", "stash me")

	out, err := StashPush(dir, "my-stash", false)
	if err != nil {
		t.Fatalf("StashPush: %v\n%s", err, out)
	}
	if files := listFiles(t, dir); len(files) != 0 {
		t.Fatalf("expected clean tree after stash all, got %v", files)
	}
	if list := stashList(t, dir); !strings.Contains(list, "my-stash") {
		t.Fatalf("expected stash named my-stash, got: %q", list)
	}
}

func TestStashPush_AllWithoutName(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	writeFile(t, dir, "README.md", "stash me")

	out, err := StashPush(dir, "", false)
	if err != nil {
		t.Fatalf("StashPush: %v\n%s", err, out)
	}
	// git auto-generates a "WIP on <branch>" message.
	if list := stashList(t, dir); !strings.Contains(list, "WIP on") {
		t.Fatalf("expected auto-named WIP stash, got: %q", list)
	}
}

func TestStashPush_StagedOnlyLeavesUnstaged(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	// staged.txt is staged; unstaged.txt is left in the working tree.
	writeFile(t, dir, "staged.txt", "staged change")
	writeFile(t, dir, "unstaged.txt", "unstaged change")
	if err := StagePath(dir, "staged.txt"); err != nil {
		t.Fatalf("StagePath: %v", err)
	}

	out, err := StashPush(dir, "staged-stash", true)
	if err != nil {
		t.Fatalf("StashPush staged: %v\n%s", err, out)
	}

	files := listFiles(t, dir)
	if len(files) != 1 || files[0].Path != "unstaged.txt" {
		t.Fatalf("expected only unstaged.txt to remain, got %v", files)
	}
	if list := stashList(t, dir); !strings.Contains(list, "staged-stash") {
		t.Fatalf("expected stash named staged-stash, got: %q", list)
	}
}

func TestStashPush_StagedOnlyWithoutName(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	writeFile(t, dir, "staged.txt", "staged change")
	if err := StagePath(dir, "staged.txt"); err != nil {
		t.Fatalf("StagePath: %v", err)
	}

	out, err := StashPush(dir, "", true)
	if err != nil {
		t.Fatalf("StashPush staged: %v\n%s", err, out)
	}
	if list := stashList(t, dir); !strings.Contains(list, "WIP on") {
		t.Fatalf("expected auto-named WIP stash, got: %q", list)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func listFiles(t *testing.T, dir string) []StageFileStatus {
	t.Helper()
	files, err := ListStageFiles(dir)
	if err != nil {
		t.Fatalf("ListStageFiles: %v", err)
	}
	return files
}

func stashList(t *testing.T, dir string) string {
	t.Helper()
	stdout, stderr, err := run(dir, []string{"stash", "list"})
	if err != nil {
		t.Fatalf("stash list: %v\n%s", err, joinOutput(stdout, stderr))
	}
	return stdout
}

func TestIsIndexLockBusyErr_Nil(t *testing.T) {
	if isIndexLockBusyErr(nil, "") {
		t.Error("expected false for nil error")
	}
}

func TestIsIndexLockBusyErr_IndexLock(t *testing.T) {
	err := &RunError{Stderr: "fatal: Unable to create 'path/index.lock'"}
	if !isIndexLockBusyErr(err, "") {
		t.Error("expected true for index.lock error")
	}
}

func TestIsIndexLockBusyErr_AnotherProcess(t *testing.T) {
	err := &RunError{Stderr: ""}
	if !isIndexLockBusyErr(err, "Another git process seems to be running") {
		t.Error("expected true for 'another git process' message")
	}
}

func TestRebase_Simple(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	// Rebase HEAD onto HEAD should be a no-op
	out, err := Rebase(dir, "HEAD")
	if err != nil {
		t.Fatalf("Rebase HEAD onto HEAD: %v\n%s", err, out)
	}
}
