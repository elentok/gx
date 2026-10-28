package server_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// atMergeCap adds three done merge tickets to an epic's tickets: its run has
// used them all, so its next non-ff pauses the root instead of forking.
func atMergeCap(opts map[string]servertest.TicketOpts) map[string]servertest.TicketOpts {
	opts["02"] = servertest.TicketOpts{Type: "merge", Status: "done"}
	opts["03"] = servertest.TicketOpts{Type: "merge", Status: "done"}
	opts["04"] = servertest.TicketOpts{Type: "merge", Status: "done"}
	return opts
}

// moveMain commits on main in repo, so a branch cut before it no longer fast-forwards.
func moveMain(t *testing.T, repo, name string) {
	t.Helper()
	testutil.WriteFile(t, repo, "main-"+name+".txt", name)
	testutil.CommitAll(t, repo, "main moves on "+name)
}

func isMergeTurn(p servertest.Prompt) bool { return strings.HasPrefix(p.Text, "/gx-merge ") }

// rebaseAndFinish is a merge ticket's turn: rebase the epic, checked out in
// p.Cwd, onto main and report finished.
func rebaseAndFinish(t *testing.T, store string, p servertest.Prompt) {
	t.Helper()
	gitOut(t, p.Cwd, "rebase", "-q", "main")
	a, err := tickets.ParseAddress(p.Address, tickets.AddressContext{})
	if err != nil {
		t.Error(err)
		return
	}
	files, _ := filepath.Glob(filepath.Join(store, a.Project, a.Epic, "issues", a.ID+"-*.md"))
	if len(files) != 1 {
		t.Errorf("ticket %s files = %v", p.Address, files)
		return
	}
	reportIteration(t, files[0], "finished")
}

// forkFixture is a schedFixture whose epic's land is a non-ff: ticket 01's
// turn moves main on. Any other epic's ticket just commits.
type forkFixture struct {
	*schedFixture
	merges, others atomic.Int32
}

// newForkFixture builds a forkFixture whose merge ticket n runs merged(n) after its rebase.
func newForkFixture(t *testing.T, merged func(f *forkFixture, n int)) *forkFixture {
	t.Helper()
	return newForkFixtureWith(t, map[string]servertest.TicketOpts{"01": {}}, merged)
}

// newForkFixtureWith is newForkFixture over the epic's tickets opts.
func newForkFixtureWith(t *testing.T, opts map[string]servertest.TicketOpts, merged func(f *forkFixture, n int)) *forkFixture {
	t.Helper()
	ff := &forkFixture{}
	ff.schedFixture = newSchedFixture(t, opts, func(f *schedFixture, p servertest.Prompt, id string) {
		switch {
		case !strings.HasPrefix(p.Address, "proj:"+schedEpic+"/"):
			ff.others.Add(1)
			commitWork(t, p, "other")
		case !isMergeTurn(p):
			commitWork(t, p, id)
			moveMain(t, f.repo, id)
		default:
			gitOut(t, p.Cwd, "rebase", "-q", "main")
			if n := int(ff.merges.Add(1)); merged != nil {
				merged(ff, n)
			}
			reportIteration(t, f.ticketFile(t, id), "finished")
		}
	})
	return ff
}

func (f *schedFixture) mergeTicketFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(f.issues, "*-merge.md"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func (f *schedFixture) rootQueued(t *testing.T) bool {
	t.Helper()
	q, err := f.h.Client.QueueItems(context.Background())
	if err != nil {
		t.Error(err)
		return false
	}
	return slices.ContainsFunc(q, func(e server.QueueItem) bool { return e.Address == "proj:"+schedEpic+"/01" })
}

func TestMergeFork_NonFFForksAMergeTicketWhoseFinishMergesTheEpic(t *testing.T) {
	t.Parallel()
	f := newForkFixture(t, func(f *forkFixture, _ int) {
		if !f.rootQueued(t) {
			t.Error("root dequeued while its merge ticket runs")
		}
		q, err := f.h.Client.QueueItems(context.Background())
		if err != nil {
			t.Error(err)
		}
		for _, it := range q {
			if it.EnqueuedAt.IsZero() {
				t.Errorf("%s queued with no enqueue time", it.Address)
			}
		}
		if md := f.epicMD(t); strings.Contains(md, "completed_at:") || strings.Contains(md, "merged_at:") {
			t.Errorf("epic stamped while its merge ticket runs:\n%s", md)
		}
		if n := f.others.Load(); n != 0 {
			t.Errorf("whole-epic dependent launched %d times before the merge", n)
		}
	})
	servertest.WriteTicketWith(t, f.store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{schedEpic}})
	f.h.Server.Rescan()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := f.h.Client.QueueAdd(ctx, "proj:dep/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add dep: %+v, %v", res, err)
	}
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "proj:"+schedEpic)

	if got := f.merges.Load(); got != 1 {
		t.Errorf("merge turns = %d, want 1", got)
	}
	files := f.mergeTicketFiles(t)
	if len(files) != 1 {
		t.Fatalf("merge tickets = %v, want one", files)
	}
	body := readFile(t, files[0])
	if strings.Contains(body, "parent:") || !strings.Contains(body, "type: merge") || !strings.Contains(body, "status: done") {
		t.Errorf("merge ticket is not a done top-level merge ticket:\n%s", body)
	}
	f.assertStamps(t, true)
	wantAddr := "original_address: proj:" + schedEpic + "/" + strings.TrimSuffix(filepath.Base(files[0]), "-merge.md")
	if !strings.Contains(body, "created_at:") || !strings.Contains(body, wantAddr) {
		t.Errorf("merge ticket lacks created_at or %q:\n%s", wantAddr, body)
	}
	if got := f.git(t, "rev-parse", "main"); got != f.git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the rebased feature branch tip", got)
	}
	if f.rootQueued(t) {
		t.Error("root still queued after the merge")
	}
	waitUntil(t, "the dependent to launch", func() bool { return f.others.Load() > 0 })
}

func TestMergeFork_TrunkMovingTwiceForksAThirdThenPausesAtTheCap(t *testing.T) {
	t.Parallel()
	f := newForkFixture(t, nil)
	// Trunk moves between each merge ticket's finish and its merge: moving it
	// inside the agent's turn would fail the finish check instead.
	var merges atomic.Int32
	f.h.Server.SetMergeInto(func(repo git.Repo, wts []git.Worktree, branch, target string) (git.MergeOutcome, error) {
		if n := merges.Add(1); n > 1 {
			moveMain(t, f.repo, fmt.Sprint("land-", n))
		}
		return git.MergeBranchInto(repo, wts, branch, target)
	})

	ev := f.pauseRoot(t)

	if want := "needs rebase: " + schedEpic + " onto main; rebase it, then merge again"; ev.Kind != string(events.NeedsRebase) || ev.Message != want {
		t.Errorf("pause = kind %q, reason %q, want kind %q, reason %q", ev.Kind, ev.Message, events.NeedsRebase, want)
	}
	if got := f.merges.Load(); got != 3 {
		t.Errorf("merge turns = %d, want 3", got)
	}
	files := f.mergeTicketFiles(t)
	if len(files) != 3 {
		t.Fatalf("merge tickets = %v, want three", files)
	}
	for _, path := range files {
		if body := readFile(t, path); !strings.Contains(body, "status: done") {
			t.Errorf("merge ticket not done:\n%s", body)
		}
	}
	f.assertRootQueued(t)
	f.assertStamps(t, false)
}

func TestMergeFork_DirtyTrunkAfterAMergeTicketPausesWithoutAnother(t *testing.T) {
	t.Parallel()
	f := newForkFixture(t, func(f *forkFixture, _ int) {
		// An untracked file in main's checkout that the merge would overwrite.
		testutil.WriteFile(t, f.repo, "01.txt", "local")
	})

	ev := f.pauseRoot(t)

	if ev.Kind != string(events.DirtyTrunk) {
		t.Errorf("kind = %q, want %q (%s)", ev.Kind, events.DirtyTrunk, ev.Message)
	}
	time.Sleep(200 * time.Millisecond) // a second merge ticket would follow within a few polls
	files := f.mergeTicketFiles(t)
	if len(files) != 1 {
		t.Fatalf("merge tickets = %v, want one", files)
	}
	if body := readFile(t, files[0]); !strings.Contains(body, "status: done") {
		t.Errorf("merge ticket not done:\n%s", body)
	}
	if got := f.merges.Load(); got != 1 {
		t.Errorf("merge turns = %d, want 1", got)
	}
	f.assertRootQueued(t)
}
