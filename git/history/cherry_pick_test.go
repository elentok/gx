package history_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/testutil"
)

func TestCherryPickRange_AppliesCommitsInOrder(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "iter")
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	testutil.WriteFile(t, dir, "b.txt", "b\n")
	testutil.CommitAll(t, dir, "add b")
	tip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "feature", base)

	if err := history.CherryPickRange(dir, base, tip); err != nil {
		t.Fatalf("CherryPickRange: %v", err)
	}

	out, err := exec.Command("git", "-C", dir, "log", "--format=%s", "-2").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	log := string(out)
	if !strings.Contains(log, "add a") || !strings.Contains(log, "add b") {
		t.Errorf("log after cherry-pick = %q, want both commits", log)
	}
}

func TestCherryPickRange_ConflictReturnsError(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepoWithConflictSetup(t)

	base, err := git.RevParse(dir, "HEAD~1")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	tip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "feature", base)
	testutil.WriteFile(t, dir, "a.txt", "line from feature\n")
	testutil.CommitAll(t, dir, "feature change")

	if err := history.CherryPickRange(dir, base, tip); err == nil {
		t.Fatal("CherryPickRange() error = nil, want conflict error")
	}

	inProgress, err := history.CherryPickInProgress(dir)
	if err != nil {
		t.Fatalf("CherryPickInProgress: %v", err)
	}
	if !inProgress {
		t.Error("CherryPickInProgress() = false, want true while mid-conflict")
	}
}

func TestAbortCherryPick_ClearsConflictAndRestoresHead(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepoWithConflictSetup(t)

	base, err := git.RevParse(dir, "HEAD~1")
	if err != nil {
		t.Fatalf("RevParse base: %v", err)
	}
	tip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse tip: %v", err)
	}
	testutil.MustGitExported(t, dir, "checkout", "-b", "feature", base)
	testutil.WriteFile(t, dir, "a.txt", "line from feature\n")
	testutil.CommitAll(t, dir, "feature change")
	featureHead, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse feature: %v", err)
	}

	if err := history.CherryPickRange(dir, base, tip); err == nil {
		t.Fatal("CherryPickRange() error = nil, want conflict error")
	}
	if err := history.AbortCherryPick(dir); err != nil {
		t.Fatalf("AbortCherryPick: %v", err)
	}
	inProgress, err := history.CherryPickInProgress(dir)
	if err != nil {
		t.Fatalf("CherryPickInProgress: %v", err)
	}
	if inProgress {
		t.Error("CherryPickInProgress() = true after abort")
	}
	gotHead, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse after abort: %v", err)
	}
	if gotHead != featureHead {
		t.Errorf("HEAD after abort = %s, want %s", gotHead, featureHead)
	}
}

func TestAbortRebase_ClearsConflictAndRestoresHead(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepoWithConflictSetup(t)

	base, err := git.RevParse(dir, "HEAD~1")
	if err != nil {
		t.Fatalf("RevParse base: %v", err)
	}
	trunk, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse trunk: %v", err)
	}
	testutil.MustGitExported(t, dir, "checkout", "-b", "feature", base)
	testutil.WriteFile(t, dir, "a.txt", "line from feature\n")
	testutil.CommitAll(t, dir, "feature change")
	featureHead, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse feature: %v", err)
	}

	if _, err := git.Rebase(dir, trunk); err == nil {
		t.Fatal("Rebase() error = nil, want conflict error")
	}
	if inProgress, _ := history.RebaseInProgress(dir); !inProgress {
		t.Fatal("RebaseInProgress() = false, want true mid-conflict")
	}
	if err := history.AbortRebase(dir); err != nil {
		t.Fatalf("AbortRebase: %v", err)
	}
	if inProgress, _ := history.RebaseInProgress(dir); inProgress {
		t.Error("RebaseInProgress() = true after abort")
	}
	if gotHead, _ := git.RevParse(dir, "HEAD"); gotHead != featureHead {
		t.Errorf("HEAD after abort = %s, want %s", gotHead, featureHead)
	}
}

func TestPatchesApplied_RangeAlreadyOnUpstream_IsApplied(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	if applied, err := history.PatchesApplied(dir, "HEAD", base, base); err != nil || applied {
		t.Fatalf("PatchesApplied(empty range) = %v, %v; want false, nil", applied, err)
	}

	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	tip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	applied, err := history.PatchesApplied(dir, "HEAD", base, tip)
	if err != nil {
		t.Fatalf("PatchesApplied: %v", err)
	}
	if !applied {
		t.Error("PatchesApplied(a range upstream already reaches) = false, want true")
	}
}

func TestTrailerMap_ParsesValueToSHA(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	if err := history.AppendTrailer(dir, "Ralph-Loop-Ticket", "epic/01"); err != nil {
		t.Fatalf("AppendTrailer: %v", err)
	}
	stamped, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.WriteFile(t, dir, "b.txt", "b\n")
	testutil.CommitAll(t, dir, "add b (no trailer)")

	got, err := history.TrailerMap(dir, "HEAD", "Ralph-Loop-Ticket")
	if err != nil {
		t.Fatalf("TrailerMap: %v", err)
	}
	want := map[string]string{"epic/01": stamped}
	if len(got) != len(want) || got["epic/01"] != want["epic/01"] {
		t.Errorf("TrailerMap() = %v, want %v", got, want)
	}
}

func TestTrailerMap_NoMatchingTrailers_EmptyMap(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	got, err := history.TrailerMap(dir, "HEAD", "Ralph-Loop-Ticket")
	if err != nil {
		t.Fatalf("TrailerMap: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("TrailerMap() = %v, want empty map", got)
	}
}

// TestCherryPickInProgress_TrueWithSequencerLeftoverButNoHead reproduces a
// state seen in production: something removes CHERRY_PICK_HEAD (e.g. a
// prior --abort that didn't run to completion) but leaves
// .git/sequencer/todo behind. Git itself still refuses a new cherry-pick in
// that state, so CherryPickInProgress must too, or callers wrongly skip the
// abort and every subsequent cherry-pick fails with "already in progress".
func TestCherryPickInProgress_TrueWithSequencerLeftoverButNoHead(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepoWithConflictSetup(t)

	base, err := git.RevParse(dir, "HEAD~1")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	tip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	testutil.MustGitExported(t, dir, "checkout", "-b", "feature", base)
	testutil.WriteFile(t, dir, "a.txt", "line from feature\n")
	testutil.CommitAll(t, dir, "feature change")

	if err := history.CherryPickRange(dir, base, tip); err == nil {
		t.Fatal("CherryPickRange() error = nil, want conflict error")
	}

	// Simulate an interrupted abort: CHERRY_PICK_HEAD is gone, but the
	// sequencer directory (todo/head/abort-safety) survives.
	if err := os.Remove(filepath.Join(dir, ".git", "CHERRY_PICK_HEAD")); err != nil {
		t.Fatalf("removing CHERRY_PICK_HEAD: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "sequencer", "todo")); err != nil {
		t.Fatalf("sequencer/todo missing before test even begins: %v", err)
	}

	inProgress, err := history.CherryPickInProgress(dir)
	if err != nil {
		t.Fatalf("CherryPickInProgress: %v", err)
	}
	if !inProgress {
		t.Error("CherryPickInProgress() = false, want true when sequencer/todo survives without CHERRY_PICK_HEAD")
	}

	if err := history.AbortCherryPick(dir); err != nil {
		t.Fatalf("AbortCherryPick: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "sequencer")); !os.IsNotExist(err) {
		t.Errorf("sequencer dir still present after AbortCherryPick, stat err = %v", err)
	}
}

func TestCherryPickInProgress_FalseOutsideCherryPick(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	inProgress, err := history.CherryPickInProgress(dir)
	if err != nil {
		t.Fatalf("CherryPickInProgress: %v", err)
	}
	if inProgress {
		t.Error("CherryPickInProgress() = true, want false outside a cherry-pick")
	}
}

func TestSquashApplied(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	got, err := history.SquashApplied(dir, "HEAD", base, "HEAD")
	if err != nil || got {
		t.Fatalf("empty range = %v, %v; want false, nil", got, err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "feat")
	for _, f := range []string{"a.txt", "b.txt"} {
		testutil.WriteFile(t, dir, f, f+"\n")
		testutil.CommitAll(t, dir, "add "+f)
	}
	testutil.MustGitExported(t, dir, "checkout", "-")
	got, err = history.SquashApplied(dir, "HEAD", base, "feat")
	if err != nil || got {
		t.Fatalf("before squash = %v, %v; want false, nil", got, err)
	}

	testutil.MustGitExported(t, dir, "merge", "--squash", "feat")
	testutil.CommitAll(t, dir, "squash")
	got, err = history.SquashApplied(dir, "HEAD", base, "feat")
	if err != nil || !got {
		t.Fatalf("after squash = %v, %v; want true, nil", got, err)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func TestStampTrailerRange_StampsEveryCommitKeepingTheRest(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	start := gitOut(t, dir, "rev-parse", "HEAD")
	testutil.MustGitExported(t, dir, "checkout", "-b", "feature")
	for i, f := range []string{"a.txt", "b.txt", "c.txt"} {
		testutil.WriteFile(t, dir, f, f+"\n")
		testutil.MustGitExported(t, dir, "add", ".")
		testutil.MustGitExported(t, dir, "commit", "-m", "add "+f, "--author", "Other Dev <other@example.com>", "--date", "2020-01-0"+string(rune('1'+i))+"T10:00:00Z")
	}
	format := "--format=%s|%an|%ae|%ad|%cd|%T"
	before := gitOut(t, dir, "log", "--reverse", "--date=raw", format, start+"..HEAD")

	tr := history.Trailer{Key: "Ralph-Loop-Ticket", Value: "gx:e/01"}
	if err := history.StampTrailerRange(dir, start, tr); err != nil {
		t.Fatalf("StampTrailerRange: %v", err)
	}

	if after := gitOut(t, dir, "log", "--reverse", "--date=raw", format, start+"..HEAD"); after != before {
		t.Errorf("subjects, authors, dates or trees changed:\n%s\nwant\n%s", after, before)
	}
	if got := gitOut(t, dir, "rev-list", "--count", "--grep=^Ralph-Loop-Ticket: gx:e/01$", start+"..HEAD"); got != "3" {
		t.Errorf("commits with the trailer = %s, want 3", got)
	}
	for body := range strings.SplitSeq(gitOut(t, dir, "log", "--format=%B%x00", start+"..HEAD"), "\x00") {
		if n := strings.Count(body, "Ralph-Loop-Ticket:"); n > 1 {
			t.Errorf("trailer appears %d times in %q", n, body)
		}
	}
	if got := gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature" {
		t.Errorf("HEAD on %q, want feature", got)
	}
	if st := gitOut(t, dir, "status", "--porcelain"); st != "" {
		t.Errorf("worktree dirty after rewrite: %q", st)
	}
	if got := gitOut(t, dir, "rev-parse", "HEAD~3"); got != start {
		t.Errorf("commits before the range changed")
	}

	// A repeat stamps nothing more.
	tip := gitOut(t, dir, "rev-parse", "HEAD")
	if err := history.StampTrailerRange(dir, start, tr); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if got := gitOut(t, dir, "rev-parse", "HEAD"); got != tip {
		t.Errorf("repeat moved HEAD")
	}
}

func TestStampTrailerRange_EmptyRangeChangesNothing(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	head := gitOut(t, dir, "rev-parse", "HEAD")
	if err := history.StampTrailerRange(dir, head, history.Trailer{Key: "K", Value: "v"}); err != nil {
		t.Fatalf("StampTrailerRange: %v", err)
	}
	if got := gitOut(t, dir, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD moved on an empty range")
	}
}

func TestStampTrailerRange_BadStartLeavesBranchUnchanged(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	head := gitOut(t, dir, "rev-parse", "HEAD")
	if err := history.StampTrailerRange(dir, "no-such-ref", history.Trailer{Key: "K", Value: "v"}); err == nil {
		t.Fatal("StampTrailerRange succeeded for a bad start")
	}
	if got := gitOut(t, dir, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD moved after a failure")
	}
}
