package server_test

import (
	"context"
	"github.com/elentok/gx/server/wire"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func samePath(t *testing.T, a, b string) bool {
	t.Helper()
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		t.Fatalf("resolve %q, %q: %v, %v", a, b, errA, errB)
	}
	return ra == rb
}

func TestTrunkConflict_TheAgentRunsInTheTargetCheckoutAndTheEpicLands(t *testing.T) {
	t.Parallel()
	cwds := make(chan string, 1)
	var f *schedFixture
	f = stashMergeFixtureWith(t, func(_ *schedFixture, p *servertest.Turn) {
		cwds <- p.Cwd
		testutil.WriteFile(t, p.Cwd, "same.txt", "resolved")
		reportIteration(t, f.TicketFile(t, "02"), "finished")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.Subscribe(ctx, t)

	if res, err := f.H.Client.EpicStashMerge(ctx, "proj:"+schedEpic); err != nil || res.Refused {
		t.Fatalf("stash-merge = %+v, %v", res, err)
	}

	select {
	case cwd := <-cwds:
		if !samePath(t, cwd, f.Repo) {
			t.Errorf("agent cwd = %s, want the target checkout %s", cwd, f.Repo)
		}
	case <-ctx.Done():
		t.Fatal("the trunk-conflict ticket was never claimed")
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "proj:"+schedEpic)
	if body := servertest.ReadFile(t, f.TicketFile(t, "02")); !strings.Contains(body, "status: done") {
		t.Errorf("trunk-conflict ticket not done:\n%s", body)
	}
}

func TestTrunkConflict_AnotherEpicsLandWaitsUnpausedUntilTheTicketIsDone(t *testing.T) {
	t.Parallel()
	// The conflict agent reports nothing, so its ticket pauses and stays open.
	f := stashMergeFixtureWith(t, func(_ *schedFixture, p *servertest.Turn) {
		if p.Address == "proj:"+schedEpic+"/02" {
			return
		}
		testutil.WriteFile(t, p.Cwd, "other.txt", "other")
		testutil.CommitAll(t, p.Cwd, "other work")
	})
	servertest.WriteTicketWith(t, f.Store, "proj", "other", "01", "t", servertest.TicketOpts{})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.Subscribe(ctx, t)
	if res, err := f.H.Client.EpicStashMerge(ctx, "proj:"+schedEpic); err != nil || res.Refused {
		t.Fatalf("stash-merge = %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventIterationPaused, "proj:"+schedEpic+"/02")

	if res, err := f.H.Client.QueueAdd(ctx, "proj:other/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventTicketDone, "proj:other/01")
	otherMerged := func() bool {
		out := f.Git(t, "ls-tree", "-r", "--name-only", "main")
		return strings.Contains(out, "other.txt")
	}
	time.Sleep(200 * time.Millisecond) // negative wait: a landing would come within a few 50ms polls
	if otherMerged() {
		t.Fatal("other landed while the trunk-conflict ticket was open")
	}
	q, err := f.H.Client.Queue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	queued := false
	for _, e := range q {
		queued = queued || e.Address == "proj:other/01"
	}
	if !queued {
		t.Errorf("other left the queue while waiting: %+v", q)
	}
	md, _ := os.ReadFile(filepath.Join(f.Store, "proj", "other", "ticket.md"))
	if strings.Contains(string(md), "land_block") {
		t.Errorf("other paused while waiting:\n%s", md)
	}

	// A person finishes the ticket by hand.
	path := f.TicketFile(t, "02")
	body := strings.Replace(servertest.ReadFile(t, path), "status: paused\n", "status: done\n", 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "proj:other")
	if !otherMerged() {
		t.Error("other did not land after the ticket was done")
	}
}
