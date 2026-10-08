package server_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func TestLandRoot_FastForwardLandsOntoTheTarget(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	if got := f.git(t, "rev-parse", "main"); got != f.git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the feature branch tip", got)
	}
	if got := f.git(t, "show", "main:01.txt"); got != "01" {
		t.Errorf("main:01.txt = %q", got)
	}
}

func TestLandRoot_AutoFFMergeOffParksTheRootUnmerged(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
	})
	servertest.SetProjectRepoNoAutoMerge(t, f.store, "proj", f.repo) // read live, so after Start is fine
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootParked, "")

	if out := f.git(t, "branch", "--contains", schedEpic, "--list", "main"); strings.TrimSpace(out) != "" {
		t.Errorf("main contains the feature branch: %q", out)
	}
}

// The all-done gate, not the merge core, keeps an epic with an open review ticket from fast-forwarding.
func TestLandRoot_OpenCodeReviewTicketHoldsTheMergeUntilItIsDone(t *testing.T) {
	var mergedDuringReview atomic.Bool
	f := newSchedFixture(t, map[string]servertest.TicketOpts{
		"01": {},
		"02": {Type: "code-review", BlockedBy: []string{"01"}},
	}, func(f *schedFixture, p servertest.Prompt, id string) {
		if id == "02" {
			// 01 has landed on the feature branch; 02 is still open and running.
			out := f.git(t, "branch", "--contains", schedEpic, "--list", "main")
			mergedDuringReview.Store(strings.TrimSpace(out) != "")
		}
		commitWork(t, p, id)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	if mergedDuringReview.Load() {
		t.Error("main contained the feature branch while the code-review ticket was open")
	}
	if got := f.git(t, "rev-parse", "main"); got != f.git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the feature branch tip once the review is done", got)
	}
	f.assertLanded(t, "01", "02")
}

func TestLandRoot_NeedsRebaseParksTheRootWithoutRebasing(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		testutil.WriteFile(t, f.repo, "moved.txt", "moved")
		testutil.CommitAll(t, f.repo, "main moves on")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootParked, "")

	main := strings.TrimSpace(f.git(t, "rev-parse", "main"))
	if got := strings.TrimSpace(f.git(t, "log", "-1", "--format=%s", "main")); got != "main moves on" {
		t.Errorf("main head = %q (%s), want it untouched", got, main)
	}
	if out := f.git(t, "branch", "--contains", schedEpic, "--list", "main"); strings.TrimSpace(out) != "" {
		t.Errorf("main contains the feature branch: %q", out)
	}
}

func TestLandRoot_AutoMergeOffLeavesTheBranchAndDequeuesTheRoot(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
	}, func(c *server.Config) { c.AutoMergeEpic = false })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	if out := f.git(t, "branch", "--contains", schedEpic, "--list", "main"); strings.TrimSpace(out) != "" {
		t.Errorf("main contains the feature branch: %q", out)
	}
}
