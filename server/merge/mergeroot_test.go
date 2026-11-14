package merge_test

import (
	"context"
	"fmt"
	"github.com/elentok/gx/server/wire"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestMergeRoot_FastForwardMergesOntoTheTarget(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	if got := f.Git(t, "rev-parse", "main"); got != f.Git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the feature branch tip", got)
	}
	if got := f.Git(t, "show", "main:01.txt"); got != "01" {
		t.Errorf("main:01.txt = %q", got)
	}
}

func TestMergeRoot_AutoFFMergeOffLeavesTheBranchAndDequeuesTheRoot(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
	})
	servertest.SetProjectRepoNoAutoMerge(t, f.Store, "proj", f.Repo) // read live, so after Start is fine
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	if out := f.Git(t, "branch", "--contains", schedEpic, "--list", "main"); strings.TrimSpace(out) != "" {
		t.Errorf("main contains the feature branch: %q", out)
	}
}

func TestMergeRoot_PRDeliveryNeverAutoMergesEvenWithAutoFFMerge(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
	})
	if err := servertest.WriteProjectRepo(f.Store, "proj", f.Repo, `,"auto-ff-merge":true,"delivery":"pr"`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	if out := f.Git(t, "branch", "--contains", schedEpic, "--list", "main"); strings.TrimSpace(out) != "" {
		t.Errorf("a pr project merged the feature branch: %q", out)
	}
}

// The all-done gate, not the merge core, keeps an epic with an open review ticket from fast-forwarding.
func TestMergeRoot_OpenCodeReviewTicketHoldsTheMergeUntilItIsDone(t *testing.T) {
	t.Parallel()
	var mergedDuringReview atomic.Bool
	f := newSchedFixture(t, map[string]servertest.TicketOpts{
		"01": {},
		"02": {Type: "code-review", BlockedBy: []string{"01"}},
	}, func(f *schedFixture, p *servertest.Turn, id string) {
		if id == "02" {
			// 01 has merged on the feature branch; 02 is still open and running.
			out := f.Git(t, "branch", "--contains", schedEpic, "--list", "main")
			mergedDuringReview.Store(strings.TrimSpace(out) != "")
		}
		commitWork(t, p, id)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	if mergedDuringReview.Load() {
		t.Error("main contained the feature branch while the code-review ticket was open")
	}
	if got := f.Git(t, "rev-parse", "main"); got != f.Git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the feature branch tip once the review is done", got)
	}
	f.AssertMerged(t, "01", "02")
}

func TestMergeRoot_NeedsRebasePausesTheRootWithoutRebasing(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		testutil.WriteFile(t, f.Repo, "moved.txt", "moved")
		testutil.CommitAll(t, f.Repo, "main moves on")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootPaused, "")

	main := strings.TrimSpace(f.Git(t, "rev-parse", "main"))
	if got := strings.TrimSpace(f.Git(t, "log", "-1", "--format=%s", "main")); got != "main moves on" {
		t.Errorf("main head = %q (%s), want it untouched", got, main)
	}
	if out := f.Git(t, "branch", "--contains", schedEpic, "--list", "main"); strings.TrimSpace(out) != "" {
		t.Errorf("main contains the feature branch: %q", out)
	}
}

func TestMergeRoot_StampsCompletedAtWhenAutoMergeIsOff(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
	})
	servertest.SetProjectRepoNoAutoMerge(t, f.Store, "proj", f.Repo)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	f.AssertStamps(t, false)
}

func TestMergeRoot_StampsMergedAtAndCompletedAtWhenTheEpicMerges(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	f.AssertStamps(t, true)
}

func TestMergeRoot_NothingToMergeStampsBothRegardlessOfAutoFFMerge(t *testing.T) {
	t.Parallel()
	for name, autoMerge := range map[string]bool{"on": true, "off": false} {
		t.Run(name, func(t *testing.T) {
			f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, _ *servertest.Turn, id string) {
				f.FinishCommitless(t, id)
			})
			if !autoMerge {
				servertest.SetProjectRepoNoAutoMerge(t, f.Store, "proj", f.Repo)
			}
			main := f.Git(t, "rev-parse", "main")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			evs := f.QueueEpic(ctx, t, "01")
			servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

			f.AssertStamps(t, true)
			if got := f.Git(t, "rev-parse", "main"); got != main {
				t.Errorf("main moved to %s, want it untouched at %s", got, main)
			}
			e := tickets.LoadEpicTicketMD(filepath.Join(f.Store, "proj", schedEpic))
			if tip := strings.TrimSpace(main); e.RangeBase != tip || e.RangeTip != tip {
				t.Errorf("range = %q..%q, want empty at %q", e.RangeBase, e.RangeTip, tip)
			}
		})
	}
}

// handMergeBeforeCompletion is an agent turn that finishes commitless after a
// person merged trailer commits (one per trailer) into main by hand, with the
// epic's branch gone. main reports main before and after that merge.
func handMergeBeforeCompletion(t *testing.T, trailers ...string) (turn func(f *schedFixture, _ *servertest.Turn, id string), main func() (before, after string)) {
	var before, after string
	turn = func(f *schedFixture, _ *servertest.Turn, id string) {
		before = strings.TrimSpace(f.Git(t, "rev-parse", "main"))
		f.Git(t, "checkout", "-q", "-b", "handwork", "main")
		for i, tr := range trailers {
			testutil.WriteFile(t, f.Repo, fmt.Sprintf("hand%d.txt", i), "x\n")
			testutil.CommitAll(t, f.Repo, "work\n\nRalph-Loop-Ticket: "+tr)
		}
		f.Git(t, "checkout", "-q", "main")
		f.Git(t, "merge", "-q", "--ff-only", "handwork")
		f.Git(t, "branch", "-q", "-D", "handwork")
		after = strings.TrimSpace(f.Git(t, "rev-parse", "main"))
		f.FinishCommitless(t, id)
	}
	return turn, func() (string, string) { return before, after }
}

func TestMergeRoot_NothingToMergeStampsTheReflogRangeOfAnEpicMergedOutsideGx(t *testing.T) {
	t.Parallel()
	turn, main := handMergeBeforeCompletion(t, schedEpic+"/01")
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, turn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	servertest.WaitForEvent(ctx, t, f.QueueEpic(ctx, t, "01"), wire.EventRootCompleted, "")

	f.AssertStamps(t, true)
	before, after := main()
	e := tickets.LoadEpicTicketMD(filepath.Join(f.Store, "proj", schedEpic))
	if e.RangeBase != before || e.RangeTip != after {
		t.Errorf("range = %q..%q, want %q..%q", e.RangeBase, e.RangeTip, before, after)
	}
}

func TestMergeRoot_NothingToMergeStampsNoRangeWhenTheEntryHoldsAnotherEpicsCommits(t *testing.T) {
	t.Parallel()
	turn, _ := handMergeBeforeCompletion(t, schedEpic+"/01", "elsewhere/01")
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, turn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	servertest.WaitForEvent(ctx, t, f.QueueEpic(ctx, t, "01"), wire.EventRootCompleted, "")

	f.AssertStamps(t, true)
	e := tickets.LoadEpicTicketMD(filepath.Join(f.Store, "proj", schedEpic))
	if e.RangeBase != "" || e.RangeTip != "" {
		t.Errorf("range = %q..%q, want none", e.RangeBase, e.RangeTip)
	}
}

func TestMergeRoot_NothingToMergeLeavesAnEpicWithAPRUnmerged(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, _ *servertest.Turn, id string) {
		f.FinishCommitless(t, id)
	})
	if err := tickets.StampEpicFields(filepath.Join(f.Store, "proj", schedEpic), map[string]any{"pr_number": 5, "pr_url": "https://github.com/o/r/pull/5"}); err != nil {
		t.Fatal(err)
	}
	f.H.Server.Rescan()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	f.AssertStamps(t, false)
}

func TestMergeRoot_StampsOnlyCompletedAtWhenTheMergePauses(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		testutil.WriteFile(t, f.Repo, "moved.txt", "moved")
		testutil.CommitAll(t, f.Repo, "main moves on")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootPaused, "")

	f.AssertStamps(t, false)
}

// stackedFixture is a schedFixture whose epic merges into the unmerged blocker
// epic's branch, not into main.
func stackedFixture(t *testing.T) *schedFixture {
	t.Helper()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {BlockedBy: []string{servertest.StackBlocker}}}, func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		f.EnableStackedAutoMerge(t)
	})
	f.StackOnBlocker(t)
	return f
}

// Story 17 of the epic-diff spec: the stamp is the derived target's tip before
// and after the merge, here the blocker's branch.
func TestMergeRoot_StackedEpicStampsTheBlockersTipBeforeAndAfter(t *testing.T) {
	t.Parallel()
	f := stackedFixture(t)
	main, blockerBefore := f.rev(t, "main"), f.rev(t, servertest.StackBlocker)

	f.runEpic(t, wire.EventRootCompleted)

	e := tickets.LoadEpicTicketMD(filepath.Join(f.Store, "proj", schedEpic))
	if got := f.rev(t, servertest.StackBlocker); got != f.rev(t, schedEpic) || got == blockerBefore {
		t.Fatalf("blocker branch at %s, want the epic's tip %s, moved from %s", got, f.rev(t, schedEpic), blockerBefore)
	}
	if e.RangeBase != blockerBefore || e.RangeTip != f.rev(t, servertest.StackBlocker) {
		t.Errorf("range = %q..%q, want %q..%q", e.RangeBase, e.RangeTip, blockerBefore, f.rev(t, servertest.StackBlocker))
	}
	if got := f.rev(t, "main"); got != main {
		t.Errorf("main moved to %s", got)
	}
}

// Story 23 of the epic-diff spec: squash-merging the blocker rewrites its
// commits, but the stacked epic's stamp stays the one it took at its own merge.
func TestMergeRoot_SquashMergingTheBlockerLeavesTheStackedEpicsStampUnchanged(t *testing.T) {
	t.Parallel()
	f := stackedFixture(t)
	f.runEpic(t, wire.EventRootCompleted)
	stamped := tickets.LoadEpicTicketMD(filepath.Join(f.Store, "proj", schedEpic))
	if stamped.RangeBase == "" || stamped.RangeTip == "" {
		t.Fatalf("epic not stamped: %q..%q", stamped.RangeBase, stamped.RangeTip)
	}

	// Ancestry cannot see a squash, so the person marks the blocker merged.
	f.Git(t, "merge", "--squash", servertest.StackBlocker)
	testutil.CommitAll(t, f.Repo, "squash blocker")
	if err := tickets.StampEpicFields(filepath.Join(f.Store, "proj", servertest.StackBlocker), map[string]any{"merged_at": time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	f.H.Server.Rescan()

	got := tickets.LoadEpicTicketMD(filepath.Join(f.Store, "proj", schedEpic))
	if got.RangeBase != stamped.RangeBase || got.RangeTip != stamped.RangeTip {
		t.Errorf("range = %q..%q after the squash, want %q..%q", got.RangeBase, got.RangeTip, stamped.RangeBase, stamped.RangeTip)
	}
}
