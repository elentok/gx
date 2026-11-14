package merge

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// branchWith creates a branch off main with one commit per message and goes
// back to main.
func branchWith(t *testing.T, dir, branch string, messages ...string) {
	t.Helper()
	testutil.MustGitExported(t, dir, "checkout", "-b", branch, "main")
	for i, m := range messages {
		testutil.WriteFile(t, dir, branch+string(rune('a'+i))+".txt", m)
		testutil.CommitAll(t, dir, m)
	}
	testutil.MustGitExported(t, dir, "checkout", "main")
}

func ffMerge(t *testing.T, dir, branch string) (old, tip string) {
	t.Helper()
	old = gitOut(t, dir, "rev-parse", "main")
	testutil.MustGitExported(t, dir, "merge", "--ff-only", branch)
	return old, gitOut(t, dir, "rev-parse", "main")
}

func rebaseOnMain(t *testing.T, dir, branch string) {
	t.Helper()
	testutil.MustGitExported(t, dir, "rebase", "main", branch)
	testutil.MustGitExported(t, dir, "checkout", "main")
}

func wantRefusal(t *testing.T, m *ReflogMatcher, epic tickets.Epic, reason ReflogRefusalReason) {
	t.Helper()
	_, ref := m.Match(epic)
	if ref == nil || ref.Reason != reason {
		t.Fatalf("Match(%s) = %v, want refusal %s", epic.Name, ref, reason)
	}
}

func newMatcher(t *testing.T, dir string) *ReflogMatcher {
	t.Helper()
	m, err := NewReflogMatcher(dir, "main")
	if err != nil {
		t.Fatalf("NewReflogMatcher: %v", err)
	}
	return m
}

func TestReflogMatcher_ExactMatchPerEpic(t *testing.T) {
	dir := testutil.TempRepo(t)
	branchWith(t, dir, "alpha", "a1\n\nRalph-Loop-Ticket: proj:alpha/01", "a2\n\nRalph-Loop-Ticket: proj:alpha/02")
	branchWith(t, dir, "beta", "b1", "b2\n\nRalph-Loop-Ticket: proj:beta/01")
	aOld, aTip := ffMerge(t, dir, "alpha")
	rebaseOnMain(t, dir, "beta")
	bOld, bTip := ffMerge(t, dir, "beta")

	m := newMatcher(t, dir)
	for _, c := range []struct {
		name     string
		old, tip string
	}{{"alpha", aOld, aTip}, {"beta", bOld, bTip}} {
		got, ref := m.Match(tickets.Epic{Name: c.name})
		if ref != nil {
			t.Fatalf("%s: refused: %v", c.name, ref)
		}
		if got.Base != c.old || got.Tip != c.tip {
			t.Fatalf("%s: got %+v, want %s..%s", c.name, got, c.old, c.tip)
		}
	}
}

func TestReflogMatcher_LegacyValueAndRenamedEpic(t *testing.T) {
	dir := testutil.TempRepo(t)
	branchWith(t, dir, "x", "x1\n\nRalph-Loop-Ticket: legacy/01")
	lOld, lTip := ffMerge(t, dir, "x")
	branchWith(t, dir, "y", "y1\n\nRalph-Loop-Ticket: proj:old/01")
	rebaseOnMain(t, dir, "y")
	rOld, rTip := ffMerge(t, dir, "y")

	m := newMatcher(t, dir)
	got, ref := m.Match(tickets.Epic{Name: "legacy"})
	if ref != nil || got.Base != lOld || got.Tip != lTip {
		t.Fatalf("legacy: %+v %v", got, ref)
	}
	got, ref = m.Match(tickets.Epic{Name: "new", BranchName: "old"})
	if ref != nil || got.Base != rOld || got.Tip != rTip {
		t.Fatalf("renamed: %+v %v", got, ref)
	}
}

func TestReflogMatcher_HandMergeCommit(t *testing.T) {
	dir := testutil.TempRepo(t)
	branchWith(t, dir, "feat", "f1\n\nRalph-Loop-Ticket: proj:feat/01")
	testutil.WriteFile(t, dir, "main.txt", "m")
	testutil.CommitAll(t, dir, "main moves")
	old := gitOut(t, dir, "rev-parse", "main")
	testutil.MustGitExported(t, dir, "merge", "--no-ff", "-m", "merge feat", "feat")
	tip := gitOut(t, dir, "rev-parse", "main")

	got, ref := newMatcher(t, dir).Match(tickets.Epic{Name: "feat"})
	if ref != nil || got.Base != old || got.Tip != tip {
		t.Fatalf("got %+v %v, want %s..%s", got, ref, old, tip)
	}
}

func TestReflogMatcher_Refusals(t *testing.T) {
	t.Run("no trailer commits", func(t *testing.T) {
		dir := testutil.TempRepo(t)
		branchWith(t, dir, "p", "p1")
		ffMerge(t, dir, "p")
		wantRefusal(t, newMatcher(t, dir), tickets.Epic{Name: "ghost"}, ReasonNoTrailerCommits)
	})

	t.Run("no matching entry", func(t *testing.T) {
		dir := testutil.TempRepo(t)
		// The first trailer commit predates the (expired) reflog; only the
		// second sits in an entry, so no entry holds all of them.
		branchWith(t, dir, "z", "z1\n\nRalph-Loop-Ticket: proj:zed/01", "z2\n\nRalph-Loop-Ticket: proj:zed/02")
		testutil.MustGitExported(t, dir, "merge", "--ff-only", "z~1")
		testutil.MustGitExported(t, dir, "reflog", "expire", "--expire=all", "refs/heads/main")
		testutil.MustGitExported(t, dir, "merge", "--ff-only", "z")
		wantRefusal(t, newMatcher(t, dir), tickets.Epic{Name: "zed"}, ReasonNoEntry)
	})

	t.Run("merged twice", func(t *testing.T) {
		dir := testutil.TempRepo(t)
		branchWith(t, dir, "tw", "t1\n\nRalph-Loop-Ticket: proj:twice/01")
		start := gitOut(t, dir, "rev-parse", "main")
		ffMerge(t, dir, "tw")
		testutil.MustGitExported(t, dir, "reset", "--hard", start)
		ffMerge(t, dir, "tw")
		wantRefusal(t, newMatcher(t, dir), tickets.Epic{Name: "twice"}, ReasonMergedTwice)
	})

	t.Run("entry holds another epic", func(t *testing.T) {
		dir := testutil.TempRepo(t)
		branchWith(t, dir, "two", "e1\n\nRalph-Loop-Ticket: proj:one/01", "e2\n\nRalph-Loop-Ticket: proj:two/01")
		ffMerge(t, dir, "two")
		wantRefusal(t, newMatcher(t, dir), tickets.Epic{Name: "one"}, ReasonSharedEntry)
	})

	t.Run("spread over two entries", func(t *testing.T) {
		dir := testutil.TempRepo(t)
		branchWith(t, dir, "sp", "s1\n\nRalph-Loop-Ticket: proj:spread/01", "s2\n\nRalph-Loop-Ticket: proj:spread/02")
		testutil.MustGitExported(t, dir, "merge", "--ff-only", "sp~1")
		testutil.MustGitExported(t, dir, "merge", "--ff-only", "sp")
		wantRefusal(t, newMatcher(t, dir), tickets.Epic{Name: "spread"}, ReasonSpread)
	})
}

func TestReflogMatcher_NoReflog(t *testing.T) {
	dir := testutil.TempRepo(t)
	branchWith(t, dir, "q", "q1\n\nRalph-Loop-Ticket: proj:quiet/01")
	ffMerge(t, dir, "q")
	testutil.MustGitExported(t, dir, "reflog", "expire", "--expire=all", "--all")
	wantRefusal(t, newMatcher(t, dir), tickets.Epic{Name: "quiet"}, ReasonNoEntry)
}
