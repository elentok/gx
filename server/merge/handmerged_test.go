package merge_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestHandMerged_PlainGitMergeReleasesAnOnMergedDependent(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	commitOnBranch(t, repo, "blocker")
	writeCompleteEpic(t, store, "blocker")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"blocker"}})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	launched := make(chan *servertest.Turn, 1)
	h.OnTurn(func(p *servertest.Turn) {
		commitWork(t, p, "dep")
		launched <- p
	})
	if res, err := h.Client.Queue.Add(context.Background(), "proj:dep/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	select {
	case p := <-launched:
		t.Fatalf("dependent launched before the blocker merged: %+v", p)
	case <-time.After(500 * time.Millisecond):
	}
	runGitIn(t, repo, "merge", "-q", "--ff-only", "blocker")

	select {
	case <-launched:
	case <-time.After(10 * time.Second):
		t.Fatal("dependent never launched after the hand merge")
	}
	if md := servertest.ReadFile(t, filepath.Join(store, "proj", "blocker", "ticket.md")); !strings.Contains(md, "merged_at:") {
		t.Errorf("blocker has no merged_at:\n%s", md)
	}
	if body := servertest.ReadFile(t, filepath.Join(store, "proj", "dep", "issues", "01-d.md")); !strings.Contains(body, "resolved_base: main@") {
		t.Errorf("dependent not based on trunk:\n%s", body)
	}
}

func TestHandMerged_PausedRootMergedByHandIsDequeuedWithEpicMerged(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p *servertest.Turn, _ string) {
		commitFiles(t, p, "a.txt")
		testutil.WriteFile(t, f.Repo, "a.txt", "local")
	}, chat)
	f.PauseRoot(t)

	if err := os.Remove(filepath.Join(f.Repo, "a.txt")); err != nil {
		t.Fatal(err)
	}
	f.Git(t, "merge", "-q", "--ff-only", schedEpic)

	waitUntil(t, "the root to leave the queue", func() bool {
		q, err := f.H.Client.Queue.Items(context.Background())
		return err == nil && !slices.ContainsFunc(q, func(e wire.QueueItem) bool { return e.Address == "proj:"+schedEpic+"/01" })
	})
	f.AssertStamps(t, true)
	if got := waitForChat(count, epicMergedText); got != 1 {
		t.Errorf("epic merged sent %d times, want once", got)
	}
	if got := count("1 ticket merged in "); got != 1 {
		t.Errorf("the merged totals line sent %d times, want once", got)
	}
}

func TestHandMerged_AnEpicWithAPRIsNotStampedByItsBranchBeingMerged(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	writeCompleteEpic(t, store, "prepic")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"prepic"}})
	if err := tickets.StampEpicFields(filepath.Join(store, "proj", "prepic"), map[string]any{"pr_number": 5, "pr_url": "https://github.com/o/r/pull/5"}); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	var mu sync.Mutex
	probed := false
	h.Server.Reconciler().SetMergedProbe(func(_, ancestor, _ string) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		probed = probed || ancestor == "refs/heads/prepic"
		return true, nil
	})
	// The probe says merged for any epic; the dependent makes prepic eligible, so only the PR guard holds it back.
	time.Sleep(500 * time.Millisecond) // negative wait: a stamp would land within a few polls
	mu.Lock()
	defer mu.Unlock()
	if probed {
		t.Error("a PR epic was probed for a hand merge")
	}
	if md := servertest.ReadFile(t, filepath.Join(store, "proj", "prepic", "ticket.md")); strings.Contains(md, "merged_at:") {
		t.Errorf("PR epic stamped merged_at:\n%s", md)
	}
}

func TestHandMerged_StampsLinesFromTheEpicsTrailerCommits(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	runGitIn(t, repo, "checkout", "-q", "-b", "blocker")
	testutil.WriteFile(t, repo, "a.txt", "1\n2\n3\n4\n5\n")
	testutil.CommitAll(t, repo, "work\n\nRalph-Loop-Ticket: blocker/01")
	testutil.WriteFile(t, repo, "README.md", "changed\n")
	testutil.CommitAll(t, repo, "other epic's work\n\nRalph-Loop-Ticket: elsewhere/01")
	runGitIn(t, repo, "checkout", "-q", "main")
	writeCompleteEpic(t, store, "blocker")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"blocker"}})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	runGitIn(t, repo, "merge", "-q", "--ff-only", "blocker")

	waitUntil(t, "merged_at", func() bool {
		return !tickets.LoadEpicTicketMD(filepath.Join(store, "proj", "blocker")).MergedAt.IsZero()
	})
	md := tickets.LoadEpicTicketMD(filepath.Join(store, "proj", "blocker"))
	if md.LinesAdded == nil || *md.LinesAdded != 5 || md.LinesRemoved == nil || *md.LinesRemoved != 0 {
		t.Errorf("lines = %v/%v, want 5/0", md.LinesAdded, md.LinesRemoved)
	}
}

func revParse(t *testing.T, repo string) string {
	t.Helper()
	sha, err := git.RevParse(repo, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	return sha
}

// trailerBranch creates branch off main with one commit per trailer and returns to main.
func trailerBranch(t *testing.T, repo, branch string, trailers ...string) {
	t.Helper()
	runGitIn(t, repo, "checkout", "-q", "-b", branch, "main")
	for i, tr := range trailers {
		testutil.WriteFile(t, repo, branch+strings.Repeat("x", i)+".txt", "x\n")
		testutil.CommitAll(t, repo, "work\n\nRalph-Loop-Ticket: "+tr)
	}
	runGitIn(t, repo, "checkout", "-q", "main")
}

// handMergeRange hand-merges the branches with mergeArgs and returns main before and after plus the stored blocker epic.
func handMergeRange(t *testing.T, store, repo string, epics []string, mergeArgs ...string) (before, after string, got map[string]tickets.Epic) {
	t.Helper()
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	before = revParse(t, repo)
	runGitIn(t, repo, mergeArgs...)
	after = revParse(t, repo)
	got = map[string]tickets.Epic{}
	for _, name := range epics {
		dir := filepath.Join(store, "proj", name)
		waitUntil(t, name+" merged_at", func() bool { return !tickets.LoadEpicTicketMD(dir).MergedAt.IsZero() })
		got[name] = tickets.LoadEpicTicketMD(dir)
	}
	return before, after, got
}

func TestHandMerged_FastForwardStampsTheReflogRange(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	trailerBranch(t, repo, "blocker", "blocker/01", "blocker/02")
	writeCompleteEpic(t, store, "blocker")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"blocker"}})
	before, after, got := handMergeRange(t, store, repo, []string{"blocker"}, "merge", "-q", "--ff-only", "blocker")
	if e := got["blocker"]; e.RangeBase != before || e.RangeTip != after {
		t.Errorf("range = %q..%q, want %q..%q", e.RangeBase, e.RangeTip, before, after)
	}
}

func TestHandMerged_NoFFMergeCommitMatches(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	trailerBranch(t, repo, "blocker", "blocker/01")
	testutil.WriteFile(t, repo, "moved.txt", "m\n")
	testutil.CommitAll(t, repo, "main moves on")
	writeCompleteEpic(t, store, "blocker")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"blocker"}})
	before, after, got := handMergeRange(t, store, repo, []string{"blocker"}, "merge", "-q", "--no-ff", "-m", "merge", "blocker")
	if e := got["blocker"]; e.RangeBase != before || e.RangeTip != after {
		t.Errorf("range = %q..%q, want %q..%q", e.RangeBase, e.RangeTip, before, after)
	}
}

func TestHandMerged_TwoEpicsInOneEntryGetNoRangeButBothMerged(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	trailerBranch(t, repo, "blocker", "blocker/01")
	runGitIn(t, repo, "checkout", "-q", "blocker")
	testutil.WriteFile(t, repo, "second.txt", "x\n")
	testutil.CommitAll(t, repo, "work\n\nRalph-Loop-Ticket: other/01")
	runGitIn(t, repo, "branch", "other", "blocker")
	runGitIn(t, repo, "checkout", "-q", "main")
	writeCompleteEpic(t, store, "blocker")
	writeCompleteEpic(t, store, "other")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"blocker", "other"}})
	_, _, got := handMergeRange(t, store, repo, []string{"blocker", "other"}, "merge", "-q", "--ff-only", "blocker")
	for name, e := range got {
		if e.RangeBase != "" || e.RangeTip != "" {
			t.Errorf("%s range = %q..%q, want none", name, e.RangeBase, e.RangeTip)
		}
	}
}

func TestHandMerged_EpicWithNoTrailerCommitsGetsAnEmptyRangeAtMainsTip(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	runGitIn(t, repo, "checkout", "-q", "-b", "blocker", "main")
	testutil.WriteFile(t, repo, "plain.txt", "x\n")
	testutil.CommitAll(t, repo, "no trailer")
	runGitIn(t, repo, "checkout", "-q", "main")
	writeCompleteEpic(t, store, "blocker")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"blocker"}})
	_, after, got := handMergeRange(t, store, repo, []string{"blocker"}, "merge", "-q", "--ff-only", "blocker")
	if e := got["blocker"]; e.RangeBase != after || e.RangeTip != after {
		t.Errorf("range = %q..%q, want empty at %q", e.RangeBase, e.RangeTip, after)
	}
}

func TestHandMerged_StampsSummedTotalsFromTheEpicsTickets(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	commitOnBranch(t, repo, "blocker")
	writeCompleteEpic(t, store, "blocker")
	path := filepath.Join(store, "proj", "blocker", "issues", "01-t.md")
	stamps := "\ntotal_tokens: 700\nactual_cost: 0.25\nfriction:\n  total_paused_secs: 9\n---\n\n"
	testutil.WriteFile(t, filepath.Dir(path), filepath.Base(path), strings.Replace(servertest.ReadFile(t, path), "\n---\n\n", stamps, 1))
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"blocker"}})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	runGitIn(t, repo, "merge", "-q", "--ff-only", "blocker")

	waitUntil(t, "merged_at", func() bool {
		return !tickets.LoadEpicTicketMD(filepath.Join(store, "proj", "blocker")).MergedAt.IsZero()
	})
	md := tickets.LoadEpicTicketMD(filepath.Join(store, "proj", "blocker"))
	if md.TotalTokens == nil || *md.TotalTokens != 700 || md.ActualCost != 0.25 {
		t.Errorf("tokens/cost = %v/%v, want 700/0.25", md.TotalTokens, md.ActualCost)
	}
	if md.Friction == nil || md.Friction.TotalPausedSecs != 9 {
		t.Errorf("friction = %+v, want 9 paused secs", md.Friction)
	}
}

func TestHandMerged_FrictionIncludesACancelledTicket(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	commitOnBranch(t, repo, "blocker")
	writeCompleteEpic(t, store, "blocker")
	path := filepath.Join(store, "proj", "blocker", "issues", "01-t.md")
	stamps := "\ntotal_tokens: 700\nactual_cost: 0.25\nfriction:\n  total_paused_secs: 9\n---\n\n"
	testutil.WriteFile(t, filepath.Dir(path), filepath.Base(path), strings.Replace(servertest.ReadFile(t, path), "\n---\n\n", stamps, 1))
	gone := "---\nid: \"02\"\nstatus: cancelled\ntype: implement\nfriction:\n  total_paused_secs: 4\n---\nBody.\n"
	testutil.WriteFile(t, filepath.Dir(path), "02-gone.md", gone)
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"blocker"}})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	runGitIn(t, repo, "merge", "-q", "--ff-only", "blocker")

	waitUntil(t, "merged_at", func() bool {
		return !tickets.LoadEpicTicketMD(filepath.Join(store, "proj", "blocker")).MergedAt.IsZero()
	})
	md := tickets.LoadEpicTicketMD(filepath.Join(store, "proj", "blocker"))
	if md.Friction == nil || md.Friction.TotalPausedSecs != 13 {
		t.Errorf("friction = %+v, want 13 paused secs", md.Friction)
	}
	if md.TotalTokens == nil || *md.TotalTokens != 700 {
		t.Errorf("tokens = %v, want 700", md.TotalTokens)
	}
}

func TestHandMerged_EpicWithNoQueuedRootAndNoDependentIsNotChecked(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	writeCompleteEpic(t, store, "idle")
	writeCompleteEpic(t, store, "waited")
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	var mu sync.Mutex
	var probed []string
	h.Server.Reconciler().SetMergedProbe(func(_, ancestor, _ string) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		probed = append(probed, ancestor)
		return false, nil
	})
	// The dependent arrives after the swap, so "waited" is the control that proves polls run.
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"waited"}})

	has := func(ref string) bool {
		mu.Lock()
		defer mu.Unlock()
		return slices.Contains(probed, ref)
	}
	waitUntil(t, "the waited-on epic to be checked", func() bool { return has("refs/heads/waited") })
	time.Sleep(200 * time.Millisecond) // negative wait: an out-of-scope probe would come within a few polls
	if has("refs/heads/idle") {
		t.Errorf("out-of-scope epic was checked: %v", probed)
	}
}
