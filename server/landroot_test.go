package server_test

import (
	"context"
	"os"
	"path/filepath"
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

func TestLandRoot_AutoFFMergeOffLeavesTheBranchAndDequeuesTheRoot(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
	})
	servertest.SetProjectRepoNoAutoMerge(t, f.store, "proj", f.repo) // read live, so after Start is fine
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

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
	f := newSchedFixture(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), func(f *schedFixture, p servertest.Prompt, id string) {
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

// epicMD is the epic's ticket.md, read after the root settled.
func (f *schedFixture) epicMD(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.store, "proj", schedEpic, "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// assertStamps checks completed_at is stamped and merged_at is stamped only when wantMerged.
func (f *schedFixture) assertStamps(t *testing.T, wantMerged bool) {
	t.Helper()
	md := f.epicMD(t)
	if !strings.Contains(md, "completed_at:") {
		t.Errorf("epic ticket.md has no completed_at:\n%s", md)
	}
	if got := strings.Contains(md, "merged_at:"); got != wantMerged {
		t.Errorf("epic ticket.md has merged_at = %v, want %v:\n%s", got, wantMerged, md)
	}
	if !strings.Contains(md, "status:") {
		t.Errorf("epic ticket.md lost its other keys:\n%s", md)
	}
}

func TestLandRoot_StampsCompletedAtWhenAutoMergeIsOff(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
	})
	servertest.SetProjectRepoNoAutoMerge(t, f.store, "proj", f.repo)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	f.assertStamps(t, false)
}

func TestLandRoot_StampsMergedAtAndCompletedAtWhenTheEpicLands(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	f.assertStamps(t, true)
}

// finishCommitless is the agent's turn for a ticket that needs no commit.
func (f *schedFixture) finishCommitless(t *testing.T, id string) {
	t.Helper()
	path := f.ticketFile(t, id)
	body := strings.Replace(readFile(t, path), "---\n", "---\ncommitless: true\niteration_status: finished\n", 1)
	testutil.WriteFile(t, filepath.Dir(path), filepath.Base(path), body)
}

func TestLandRoot_NothingToLandStampsBothRegardlessOfAutoFFMerge(t *testing.T) {
	for name, autoMerge := range map[string]bool{"on": true, "off": false} {
		t.Run(name, func(t *testing.T) {
			f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, _ servertest.Prompt, id string) {
				f.finishCommitless(t, id)
			})
			if !autoMerge {
				servertest.SetProjectRepoNoAutoMerge(t, f.store, "proj", f.repo)
			}
			main := f.git(t, "rev-parse", "main")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			evs := f.queueEpic(ctx, t, "01")
			servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

			f.assertStamps(t, true)
			if got := f.git(t, "rev-parse", "main"); got != main {
				t.Errorf("main moved to %s, want it untouched at %s", got, main)
			}
		})
	}
}

func TestLandRoot_StampsOnlyCompletedAtWhenTheLandParks(t *testing.T) {
	f := newSchedFixture(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		testutil.WriteFile(t, f.repo, "moved.txt", "moved")
		testutil.CommitAll(t, f.repo, "main moves on")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootParked, "")

	f.assertStamps(t, false)
}
