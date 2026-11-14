package rangediff_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/git/rangediff"
	"github.com/elentok/gx/testutil"
)

const trailerKey = "Ralph-Loop-Ticket"

var testEpics = []rangediff.EpicName{{Name: "renamed", Branch: "oldname"}, {Name: "epic", Branch: "epic"}}

func stamped(t *testing.T, dir, file, subject string, trailers ...string) {
	t.Helper()
	testutil.WriteFile(t, dir, file, subject+"\n")
	testutil.CommitAll(t, dir, subject)
	for _, v := range trailers {
		if err := history.AppendTrailer(dir, trailerKey, v); err != nil {
			t.Fatalf("AppendTrailer: %v", err)
		}
	}
}

func commitsOf(t *testing.T, dir string) rangediff.CommitList {
	t.Helper()
	list, err := rangediff.ListCommits(dir, mustResolve(t, dir, "base", "HEAD"), "proj", testEpics)
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	return list
}

func TestCommitsTrailersAreCanonical(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	mustGit(t, dir, "branch", "base")
	stamped(t, dir, "a", "modern", "proj:epic/01")
	stamped(t, dir, "b", "legacy", "epic/02")
	stamped(t, dir, "c", "renamed", "oldname/03")
	stamped(t, dir, "d", "renamed modern", "proj:oldname/03")
	stamped(t, dir, "e", "none")
	stamped(t, dir, "f", "garbage", "nonsense")
	stamped(t, dir, "g", "unknown epic", "other/04")
	stamped(t, dir, "h", "two lines", "epic/05", "epic/06")

	want := []string{"proj:epic/01", "proj:epic/02", "proj:renamed/03", "proj:renamed/03", "", "", "", ""}
	list := commitsOf(t, dir)
	if len(list.Commits) != len(want) {
		t.Fatalf("got %d commits, want %d", len(list.Commits), len(want))
	}
	for i, c := range list.Commits {
		if c.Ticket != want[i] {
			t.Errorf("%q: ticket = %q, want %q", c.Subject, c.Ticket, want[i])
		}
	}
	if list.Commits[0].Subject != "modern" || len(list.Commits[0].Files) != 1 || list.Commits[0].Time.IsZero() {
		t.Errorf("first commit = %+v", list.Commits[0])
	}
}

func TestCommitsMergeAndEmptyCommit(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	mustGit(t, dir, "branch", "base")
	mustGit(t, dir, "checkout", "-b", "side")
	stamped(t, dir, "side.txt", "side work")
	mustGit(t, dir, "checkout", "main")
	stamped(t, dir, "main.txt", "main work")
	mustGit(t, dir, "commit", "--allow-empty", "-m", "empty one")
	mustGit(t, dir, "merge", "--no-ff", "-m", "merge side", "side")

	byName := map[string]rangediff.Commit{}
	for _, c := range commitsOf(t, dir).Commits {
		byName[c.Subject] = c
	}
	if m := byName["merge side"]; !m.Merge || len(m.Files) != 0 {
		t.Errorf("merge commit = %+v, want merge with no files", m)
	}
	if e, ok := byName["empty one"]; !ok || e.Merge || len(e.Files) != 0 {
		t.Errorf("empty commit = %+v, ok=%v", e, ok)
	}
	if byName["main work"].Merge || len(byName["main work"].Files) != 1 {
		t.Errorf("plain commit = %+v", byName["main work"])
	}
}

func TestCommitsTruncatesAtCapButFileListCoversRange(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	mustGit(t, dir, "branch", "base")
	stamped(t, dir, "late.txt", "has a file")
	for i := range rangediff.MaxCommits {
		mustGit(t, dir, "commit", "--allow-empty", "-m", fmt.Sprintf("empty %d", i))
	}
	list := commitsOf(t, dir)
	if len(list.Commits) != rangediff.MaxCommits || !list.Truncated {
		t.Fatalf("got %d commits truncated=%v", len(list.Commits), list.Truncated)
	}
	if list.Commits[0].Subject != "has a file" {
		t.Errorf("first = %q, want the oldest commit", list.Commits[0].Subject)
	}
	files := listByPath(t, dir, mustResolve(t, dir, "base", "HEAD"))
	if _, ok := files["late.txt"]; !ok {
		t.Errorf("file list = %+v, want late.txt", files)
	}
}

func TestNarrowFilesAndPatchesPerCommit(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	mustGit(t, dir, "branch", "base")
	testutil.WriteFile(t, dir, "x.txt", "one\n")
	testutil.CommitAll(t, dir, "c1")
	testutil.WriteFile(t, dir, "y.txt", "y\n")
	testutil.CommitAll(t, dir, "c2 between")
	testutil.WriteFile(t, dir, "x.txt", "one\ntwo\n")
	testutil.CommitAll(t, dir, "c3")
	r := mustResolve(t, dir, "base", "HEAD")
	list := commitsOf(t, dir)
	c1, c2, c3 := list.Commits[0].SHA, list.Commits[1].SHA, list.Commits[2].SHA

	// Selection order does not matter; range order wins.
	files, err := rangediff.ListFilesIn(dir, r, []string{c3, c1})
	if err != nil {
		t.Fatalf("ListFilesIn: %v", err)
	}
	if len(files.Files) != 1 || files.Files[0].Path != "x.txt" || files.Files[0].Added != 2 {
		t.Fatalf("files = %+v, want x.txt only with 2 added lines", files.Files)
	}
	ps, err := rangediff.PatchesIn(dir, r, []string{c3, c1}, files.Files[0])
	if err != nil {
		t.Fatalf("PatchesIn: %v", err)
	}
	if len(ps) != 2 || ps[0].SHA != c1 || ps[0].Subject != "c1" || ps[1].SHA != c3 || ps[1].Subject != "c3" {
		t.Fatalf("patches = %+v, want c1 then c3", ps)
	}
	if _, err := rangediff.PatchesIn(dir, r, []string{c2}, files.Files[0]); err != nil {
		t.Errorf("unrelated commit: %v", err)
	}
}

func TestNarrowMergeCommitHasNoPatchAndUnknownSHAIsRefused(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	mustGit(t, dir, "branch", "base")
	mustGit(t, dir, "checkout", "-b", "side")
	stamped(t, dir, "side.txt", "side work")
	mustGit(t, dir, "checkout", "main")
	stamped(t, dir, "main.txt", "main work")
	mustGit(t, dir, "merge", "--no-ff", "-m", "merge side", "side")
	r := mustResolve(t, dir, "base", "HEAD")
	merge := mustGit(t, dir, "rev-parse", "HEAD")

	files, err := rangediff.ListFilesIn(dir, r, []string{merge})
	if err != nil || len(files.Files) != 0 {
		t.Fatalf("merge files = %+v err=%v, want none", files.Files, err)
	}
	ps, err := rangediff.PatchesIn(dir, r, []string{merge}, rangediff.File{Path: "side.txt", Status: "A"})
	if err != nil || len(ps) != 0 {
		t.Fatalf("merge patches = %+v err=%v, want none", ps, err)
	}

	outside := mustGit(t, dir, "rev-parse", "base")
	var ue *rangediff.UnknownCommitError
	if _, err := rangediff.ListFilesIn(dir, r, []string{outside}); !errors.As(err, &ue) {
		t.Errorf("ListFilesIn err = %v, want UnknownCommitError", err)
	}
	if _, err := rangediff.PatchesIn(dir, r, []string{outside}, rangediff.File{Path: "a"}); !errors.As(err, &ue) {
		t.Errorf("PatchesIn err = %v, want UnknownCommitError", err)
	}
}
