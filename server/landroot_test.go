package server_test

import (
	"context"
	"strings"
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
