package git

import (
	"os"
	"path/filepath"
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
