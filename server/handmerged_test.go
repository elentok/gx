package server_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// writeCompleteEpic writes epic with one done ticket and completed_at, but no merged_at.
func writeCompleteEpic(t *testing.T, store, epic string) {
	t.Helper()
	servertest.WriteTicketWith(t, store, "proj", epic, "01", "t", servertest.TicketOpts{})
	path := filepath.Join(store, "proj", epic, "issues", "01-t.md")
	testutil.WriteFile(t, filepath.Dir(path), filepath.Base(path), strings.Replace(readFile(t, path), "status: open", "status: done", 1))
	if err := tickets.StampEpicFields(filepath.Join(store, "proj", epic), map[string]any{"completed_at": time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
}

// commitOnBranch commits one file on a new branch, leaving main checked out.
func commitOnBranch(t *testing.T, repo, branch string) {
	t.Helper()
	runGitIn(t, repo, "checkout", "-q", "-b", branch)
	testutil.WriteFile(t, repo, branch+".txt", branch)
	testutil.CommitAll(t, repo, "work "+branch)
	runGitIn(t, repo, "checkout", "-q", "main")
}

func runGitIn(t *testing.T, repo string, args ...string) {
	t.Helper()
	f := schedFixture{repo: repo}
	f.git(t, args...)
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !cond(); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func TestHandMerged_PlainGitMergeReleasesAnOnMergedDependent(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	commitOnBranch(t, repo, "blocker")
	writeCompleteEpic(t, store, "blocker")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"blocker"}})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	launched := make(chan servertest.Prompt, 1)
	h.RegisterLaunch(func(p servertest.Prompt) {
		commitWork(t, p, "dep")
		launched <- p
	})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:dep/01", "claude"); err != nil || res.Refused {
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
	if md := readFile(t, filepath.Join(store, "proj", "blocker", "ticket.md")); !strings.Contains(md, "merged_at:") {
		t.Errorf("blocker has no merged_at:\n%s", md)
	}
	if body := readFile(t, filepath.Join(store, "proj", "dep", "issues", "01-d.md")); !strings.Contains(body, "resolved_base: main@") {
		t.Errorf("dependent not based on trunk:\n%s", body)
	}
}

func TestHandMerged_PausedRootMergedByHandIsDequeuedWithEpicMerged(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, _ string) {
		commitFiles(t, p, "a.txt")
		testutil.WriteFile(t, f.repo, "a.txt", "local")
	}, chat)
	f.pauseRoot(t)

	if err := os.Remove(filepath.Join(f.repo, "a.txt")); err != nil {
		t.Fatal(err)
	}
	f.git(t, "merge", "-q", "--ff-only", schedEpic)

	waitUntil(t, "the root to leave the queue", func() bool {
		q, err := f.h.Client.QueueItems(context.Background())
		return err == nil && !slices.ContainsFunc(q, func(e server.QueueItem) bool { return e.Address == "proj:"+schedEpic+"/01" })
	})
	f.assertStamps(t, true)
	if got := waitForChat(count, epicMergedText); got != 1 {
		t.Errorf("epic merged sent %d times, want once", got)
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

func TestHandMerged_StampsSummedTotalsFromTheEpicsTickets(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	commitOnBranch(t, repo, "blocker")
	writeCompleteEpic(t, store, "blocker")
	path := filepath.Join(store, "proj", "blocker", "issues", "01-t.md")
	stamps := "\ntotal_tokens: 700\nactual_cost: 0.25\nfriction:\n  total_paused_secs: 9\n---\n\n"
	testutil.WriteFile(t, filepath.Dir(path), filepath.Base(path), strings.Replace(readFile(t, path), "\n---\n\n", stamps, 1))
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
	testutil.WriteFile(t, filepath.Dir(path), filepath.Base(path), strings.Replace(readFile(t, path), "\n---\n\n", stamps, 1))
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
	h.Server.SetMergedProbe(func(_, ancestor, _ string) (bool, error) {
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
	time.Sleep(200 * time.Millisecond)
	if has("refs/heads/idle") {
		t.Errorf("out-of-scope epic was checked: %v", probed)
	}
}
