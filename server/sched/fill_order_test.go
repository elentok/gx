package sched_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// fillHarness starts a server whose agents hold their turn, so each launched
// ticket keeps its slot. epics maps an epic to its ticket ids.
func fillHarness(t *testing.T, epics map[string][]string, cfg func(*server.Config)) (*servertest.Harness, context.Context) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	for epic, ids := range epics {
		for _, id := range ids {
			servertest.WriteTicket(t, store, "proj", epic, id, "t"+id, "")
		}
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, cfg)
	h.OnTurn(func(turn *servertest.Turn) { turn.Hold() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return h, ctx
}

func queueRoots(t *testing.T, h *servertest.Harness, ctx context.Context, addrs ...string) {
	t.Helper()
	for _, addr := range addrs {
		if res, err := h.Client.Queue.Add(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
}

func runAddresses(h *servertest.Harness) []string {
	var got []string
	for _, r := range h.Server.Runs() {
		got = append(got, r.Address)
	}
	slices.Sort(got)
	return got
}

func waitRuns(t *testing.T, h *servertest.Harness, want ...string) {
	t.Helper()
	if !servertest.Poll(30*time.Second, func() bool { return slices.Equal(runAddresses(h), want) }) {
		t.Fatalf("runs = %v, want %v", runAddresses(h), want)
	}
}

// settled asserts runs stay at want for a beat, so a later wrong launch is caught.
func settled(t *testing.T, h *servertest.Harness, want ...string) {
	t.Helper()
	time.Sleep(500 * time.Millisecond)
	if got := runAddresses(h); !slices.Equal(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
}

// Fill-first: the first queued epic takes every slot before the second gets one.
func TestRunner_FillsTheFirstQueuedEpicBeforeTheNext(t *testing.T) {
	t.Parallel()
	h, ctx := fillHarness(t, map[string][]string{"epic-a": {"01", "02", "03"}, "epic-b": {"01"}}, func(c *server.Config) {
		c.MaxAgents = 2
		c.MaxAgentsPerRoot = 2
	})
	queueRoots(t, h, ctx, "proj:epic-a/01", "proj:epic-b/01")
	waitRuns(t, h, "proj:epic-a/01", "proj:epic-a/02")
	settled(t, h, "proj:epic-a/01", "proj:epic-a/02")
}

// Cap skip: an epic at its per-epic cap is skipped, so the next epic takes the
// remaining slot; reordering the queue never takes back a claimed ticket.
func TestRunner_SkipsAnEpicAtItsCapAndAReorderKeepsClaimedTickets(t *testing.T) {
	t.Parallel()
	h, ctx := fillHarness(t, map[string][]string{"epic-a": {"01", "02"}, "epic-b": {"01"}}, func(c *server.Config) {
		c.MaxAgents = 3
		c.MaxAgentsPerRoot = 1
	})
	queueRoots(t, h, ctx, "proj:epic-a/01", "proj:epic-b/01")
	waitRuns(t, h, "proj:epic-a/01", "proj:epic-b/01")

	if res, err := h.Client.Queue.Move(ctx, "proj:epic-b/01", 1); err != nil || res.Refused {
		t.Fatalf("move: %+v, %v", res, err)
	}
	settled(t, h, "proj:epic-a/01", "proj:epic-b/01")
}
