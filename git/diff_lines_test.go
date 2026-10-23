package git_test

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestDiffLines_CountsAddedAndRemovedBetweenCommits(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "a.txt", "one\ntwo\nthree\n")
	testutil.CommitAll(t, dir, "add a")
	before, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	testutil.WriteFile(t, dir, "a.txt", "one\nthree\nfour\nfive\n")
	testutil.CommitAll(t, dir, "edit a")

	added, removed, err := git.DiffLines(dir, before, "HEAD")
	if err != nil {
		t.Fatalf("DiffLines: %v", err)
	}
	if added != 2 || removed != 1 {
		t.Errorf("DiffLines() = +%d -%d, want +2 -1", added, removed)
	}
}

func TestTrailerLines_SumsOnlyCommitsWhoseTrailerMatches(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.WriteFile(t, dir, "a.txt", "one\ntwo\nthree\n")
	testutil.CommitAll(t, dir, "add a\n\nRalph-Loop-Ticket: mine/01")
	testutil.WriteFile(t, dir, "a.txt", "one\nthree\nfour\n")
	testutil.CommitAll(t, dir, "edit a\n\nRalph-Loop-Ticket: mine/02")
	testutil.WriteFile(t, dir, "b.txt", "x\ny\nz\n")
	testutil.CommitAll(t, dir, "other epic\n\nRalph-Loop-Ticket: other/01")
	testutil.WriteFile(t, dir, "c.txt", "q\n")
	testutil.CommitAll(t, dir, "no trailer")

	added, removed, err := git.TrailerLines(dir, "HEAD", "Ralph-Loop-Ticket", func(v string) bool {
		return strings.HasPrefix(v, "mine/")
	})
	if err != nil {
		t.Fatalf("TrailerLines: %v", err)
	}
	if added != 4 || removed != 1 {
		t.Errorf("TrailerLines() = +%d -%d, want +4 -1", added, removed)
	}
}

func TestDiffLines_SameRefIsZero(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	added, removed, err := git.DiffLines(dir, "HEAD", "HEAD")
	if err != nil || added != 0 || removed != 0 {
		t.Errorf("DiffLines() = +%d -%d, %v, want +0 -0", added, removed, err)
	}
}
