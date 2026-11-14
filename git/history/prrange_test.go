package history

import (
	"testing"

	"github.com/elentok/gx/testutil"
)

// prFixture is a repo with a PR branch of two commits off main.
type prFixture struct {
	dir, fork, headA, headB string
}

func newPRFixture(t *testing.T) prFixture {
	t.Helper()
	dir := testutil.TempRepo(t)
	f := prFixture{dir: dir, fork: revParse(t, dir, "main")}
	testutil.MustGitExported(t, dir, "checkout", "-q", "-b", "feature")
	testutil.WriteFile(t, dir, "a.txt", "a")
	testutil.CommitAll(t, dir, "add a")
	f.headA = revParse(t, dir, "HEAD")
	testutil.WriteFile(t, dir, "b.txt", "b")
	testutil.CommitAll(t, dir, "add b")
	f.headB = revParse(t, dir, "HEAD")
	testutil.MustGitExported(t, dir, "checkout", "-q", "main")
	return f
}

func (f prFixture) merge(t *testing.T, m PRMerge) (CommitRange, string) {
	t.Helper()
	m.TrunkTip = revParse(t, f.dir, "main")
	if m.Number == 0 {
		m.Number = 7
	}
	r, reason, err := PRRange(f.dir, m)
	if err != nil {
		t.Fatal(err)
	}
	return r, reason
}

var prHeadlines = []string{"add a", "add b"}

// rebase replays the branch on main with the given subjects.
func (f prFixture) rebase(t *testing.T, subjects ...string) (first, last string) {
	t.Helper()
	for i, sha := range []string{f.headA, f.headB} {
		testutil.MustGitExported(t, f.dir, "cherry-pick", sha)
		testutil.MustGitExported(t, f.dir, "commit", "-q", "--amend", "-m", subjects[i])
		if i == 0 {
			first = revParse(t, f.dir, "HEAD")
		}
	}
	return first, revParse(t, f.dir, "HEAD")
}

func TestPRRange_MergeCommit(t *testing.T) {
	f := newPRFixture(t)
	testutil.MustGitExported(t, f.dir, "merge", "-q", "--no-ff", "-m", "Merge pull request #7", "feature")
	m := revParse(t, f.dir, "HEAD")
	got, reason := f.merge(t, PRMerge{MergeCommit: m})
	if want := (CommitRange{Base: f.fork, Tip: m}); got != want {
		t.Fatalf("got %v (%q), want %v", got, reason, want)
	}
}

func TestPRRange_Squash(t *testing.T) {
	f := newPRFixture(t)
	testutil.MustGitExported(t, f.dir, "merge", "-q", "--squash", "feature")
	testutil.MustGitExported(t, f.dir, "commit", "-q", "-m", "Add things (#7)")
	m := revParse(t, f.dir, "HEAD")
	got, reason := f.merge(t, PRMerge{MergeCommit: m, Headlines: prHeadlines, ForkPoint: f.fork})
	if want := (CommitRange{Base: f.fork, Tip: m}); got != want {
		t.Fatalf("got %v (%q), want %v", got, reason, want)
	}
}

func TestPRRange_SquashWithoutMarkIsRefused(t *testing.T) {
	f := newPRFixture(t)
	testutil.MustGitExported(t, f.dir, "merge", "-q", "--squash", "feature")
	testutil.MustGitExported(t, f.dir, "commit", "-q", "-m", "Add things")
	got, reason := f.merge(t, PRMerge{MergeCommit: revParse(t, f.dir, "HEAD"), Headlines: prHeadlines, ForkPoint: f.fork})
	if got != (CommitRange{}) || reason == "" {
		t.Fatalf("got %v (%q), want no range and a reason", got, reason)
	}
}

func TestPRRange_Rebase(t *testing.T) {
	f := newPRFixture(t)
	first, last := f.rebase(t, "add a", "add b")
	for name, m := range map[string]PRMerge{
		"M known":   {MergeCommit: last},
		"M unknown": {},
	} {
		m.Headlines, m.ForkPoint = prHeadlines, f.fork
		got, reason := f.merge(t, m)
		if want := (CommitRange{Base: f.fork, Tip: last}); got != want {
			t.Fatalf("%s: got %v (%q), want %v (first %s)", name, got, reason, want, first)
		}
	}
}

func TestPRRange_RebaseWithRewordedHeadlinesIsRefused(t *testing.T) {
	f := newPRFixture(t)
	_, last := f.rebase(t, "reworded a", "reworded b")
	got, reason := f.merge(t, PRMerge{MergeCommit: last, Headlines: prHeadlines, ForkPoint: f.fork})
	if got != (CommitRange{}) || reason == "" {
		t.Fatalf("got %v (%q), want no range and a reason", got, reason)
	}
}

func TestPRRange_RunFoundTwiceIsRefused(t *testing.T) {
	f := newPRFixture(t)
	f.rebase(t, "add a", "add b")
	testutil.MustGitExported(t, f.dir, "commit", "-q", "--allow-empty", "-m", "add a")
	testutil.MustGitExported(t, f.dir, "commit", "-q", "--allow-empty", "-m", "add b")
	got, reason := f.merge(t, PRMerge{Headlines: prHeadlines, ForkPoint: f.fork})
	if got != (CommitRange{}) || reason == "" {
		t.Fatalf("got %v (%q), want no range and a reason", got, reason)
	}
}

func TestPRRange_BranchGoneAndMergeCommitEmpty(t *testing.T) {
	f := newPRFixture(t)
	f.rebase(t, "add a", "add b")
	got, reason := f.merge(t, PRMerge{})
	if got != (CommitRange{}) || reason == "" {
		t.Fatalf("got %v (%q), want no range and a reason", got, reason)
	}
}

func TestPRRange_BranchGoneStillStampsMergeCommitAndSquash(t *testing.T) {
	f := newPRFixture(t)
	testutil.MustGitExported(t, f.dir, "merge", "-q", "--squash", "feature")
	testutil.MustGitExported(t, f.dir, "commit", "-q", "-m", "Add things (#7)")
	m := revParse(t, f.dir, "HEAD")
	got, reason := f.merge(t, PRMerge{MergeCommit: m})
	if want := (CommitRange{Base: f.fork, Tip: m}); got != want {
		t.Fatalf("got %v (%q), want %v", got, reason, want)
	}
}
