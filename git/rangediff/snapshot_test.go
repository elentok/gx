package rangediff_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git/rangediff"
	"github.com/elentok/gx/testutil"
)

func snapshotRange(t *testing.T, dir, base string) (rangediff.Snapshot, map[string]rangediff.File) {
	t.Helper()
	s, err := rangediff.SnapshotWorktree(dir)
	if err != nil {
		t.Fatalf("SnapshotWorktree: %v", err)
	}
	return s, listByPath(t, dir, mustResolve(t, dir, base, s.SHA))
}

func TestSnapshotListsCommittedUncommittedDeletedAndUntracked(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "keep.txt", "one\n")
	testutil.WriteFile(t, dir, "gone.txt", "bye\n")
	testutil.WriteFile(t, dir, ".gitignore", "ignored.log\n")
	testutil.CommitAll(t, dir, "base")
	base := mustGit(t, dir, "rev-parse", "HEAD")
	testutil.WriteFile(t, dir, "committed.txt", "c\n")
	testutil.CommitAll(t, dir, "committed")
	testutil.WriteFile(t, dir, "keep.txt", "one\ntwo\n")
	testutil.WriteFile(t, dir, "new.txt", "n\n")
	testutil.WriteFile(t, dir, "ignored.log", "x\n")
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}

	s, files := snapshotRange(t, dir, base)

	want := map[string]string{"committed.txt": "A", "keep.txt": "M", "gone.txt": "D", "new.txt": "A"}
	for p, st := range want {
		if files[p].Status != st {
			t.Errorf("%s status = %q, want %q (files %+v)", p, files[p].Status, st, files)
		}
	}
	if _, ok := files["ignored.log"]; ok {
		t.Error("ignored file in snapshot")
	}
	if !s.Dirty || s.LeftOut != 0 {
		t.Errorf("snapshot = %+v, want dirty and nothing left out", s)
	}
}

func TestSnapshotPairsRenameAcrossCommitAndUncommittedEdit(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	body := strings.Repeat("line of text\n", 20)
	testutil.WriteFile(t, dir, "old.txt", body)
	testutil.CommitAll(t, dir, "base")
	base := mustGit(t, dir, "rev-parse", "HEAD")
	mustGit(t, dir, "mv", "old.txt", "mid.txt")
	testutil.CommitAll(t, dir, "rename")
	mustGit(t, dir, "mv", "mid.txt", "new.txt")
	testutil.WriteFile(t, dir, "new.txt", body+"extra\n")

	_, files := snapshotRange(t, dir, base)

	f, ok := files["new.txt"]
	if !ok || f.Status != "R" || f.OldPath != "old.txt" || len(files) != 1 {
		t.Errorf("files = %+v, want one rename old.txt -> new.txt", files)
	}
}

func TestSnapshotLeavesWorktreeUntouched(t *testing.T) {
	t.Parallel()
	_, dir := testutil.TempRepoWithLinkedWorktree(t, "wt")
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "a")
	testutil.WriteFile(t, dir, "a.txt", "changed\n")
	testutil.WriteFile(t, dir, "u.txt", "u\n")
	index := mustGit(t, dir, "rev-parse", "--path-format=absolute", "--git-path", "index")
	lock := index + ".lock"

	read := func() (string, string, string) {
		data, err := os.ReadFile(index)
		if err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(index)
		if err != nil {
			t.Fatal(err)
		}
		return string(data), st.ModTime().String(), mustGit(t, dir, "--no-optional-locks", "status", "--porcelain")
	}
	data0, mtime0, status0 := read()

	if _, err := rangediff.SnapshotWorktree(dir); err != nil {
		t.Fatal(err)
	}

	data1, mtime1, status1 := read()
	if data0 != data1 || mtime0 != mtime1 || status0 != status1 {
		t.Errorf("worktree changed: mtime %s -> %s, status %q -> %q, index equal %v", mtime0, mtime1, status0, status1, data0 == data1)
	}
	if _, err := os.Stat(lock); err == nil {
		t.Error("index.lock left behind")
	}
}

func TestSnapshotIgnoresHostileUserConfig(t *testing.T) {
	dir := testutil.TempRepo(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	mustGit(t, dir, "config", "commit.gpgsign", "true")
	mustGit(t, dir, "config", "gpg.program", "false")
	mustGit(t, dir, "config", "--unset", "user.name")
	mustGit(t, dir, "config", "--unset", "user.email")
	mustGit(t, dir, "config", "user.useConfigOnly", "true")
	testutil.WriteFile(t, dir, "a.txt", "a\n")

	s, err := rangediff.SnapshotWorktree(dir)
	if err != nil {
		t.Fatalf("SnapshotWorktree: %v", err)
	}
	got := mustGit(t, dir, "log", "-1", "--format=%an|%G?", s.SHA)
	if got != "gx snapshot|N" {
		t.Errorf("author|signature = %q, want %q", got, "gx snapshot|N")
	}
}

func TestSnapshotLeavesOutUntrackedAboveCap(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "tracked.txt", "t\n")
	testutil.CommitAll(t, dir, "base")
	base := mustGit(t, dir, "rev-parse", "HEAD")
	testutil.WriteFile(t, dir, "tracked.txt", "t2\n")
	const n = rangediff.MaxSnapshotUntracked + 1
	for i := range n {
		testutil.WriteFile(t, dir, fmt.Sprintf("many/f%d.txt", i), "u\n")
	}

	s, files := snapshotRange(t, dir, base)

	if s.LeftOut != n {
		t.Errorf("LeftOut = %d, want %d", s.LeftOut, n)
	}
	if len(files) != 1 || files["tracked.txt"].Status != "M" {
		t.Errorf("files = %d entries, want only tracked.txt modified", len(files))
	}
}

func TestSnapshotLeavesOutOversizedUntrackedFile(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	base := mustGit(t, dir, "rev-parse", "HEAD")
	testutil.WriteFile(t, dir, "big.bin", strings.Repeat("b", rangediff.MaxBlobBytes+1))
	testutil.WriteFile(t, dir, "small.txt", "s\n")

	s, files := snapshotRange(t, dir, base)

	if s.LeftOut != 1 {
		t.Errorf("LeftOut = %d, want 1", s.LeftOut)
	}
	if _, ok := files["big.bin"]; ok {
		t.Error("oversized file in snapshot")
	}
	if files["small.txt"].Status != "A" {
		t.Errorf("files = %+v, want small.txt added", files)
	}
}

func TestSnapshotCleanTreeIsNotDirty(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "a")

	s, err := rangediff.SnapshotWorktree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Dirty || s.LeftOut != 0 {
		t.Errorf("snapshot = %+v, want clean", s)
	}
}
