package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// startsWithLive reports whether queuing first starts an iteration while live
// agents of its root already run.
func startsWithLive(t *testing.T, h *servertest.Harness, root, first string, live int) bool {
	t.Helper()
	registerLaunch(h)
	for i := 0; i < live; i++ {
		h.Server.PutRun(root, server.Run{Address: root + "/9" + string(rune('0'+i))})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.QueueAdd(ctx, first, "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for ev := range evs {
		if ev.Type == server.EventIterationStarted {
			return true
		}
	}
	return false
}

// The one-offs epic runs up to max-one-off-agents at once, whatever the
// per-root cap says; a ticket's agent field (known, unknown) does not matter
// to the cap.
func TestRunner_OneOffsEpicUsesTheOneOffCapNotThePerRootCap(t *testing.T) {
	for _, tc := range []struct {
		oneOffCap, live int
		starts          bool
	}{{3, 2, true}, {2, 2, false}} {
		store, repo := t.TempDir(), testutil.TempRepo(t)
		servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Agent: "nonsense"})
		servertest.SetProjectRepo(t, store, "proj", repo)
		h := servertest.StartWithStore(t, store, func(c *server.Config) {
			c.MaxAgentsPerRoot = 1
			c.MaxOneOffAgents = tc.oneOffCap
			c.MaxAgents = 10
			c.PollInterval = 50 * time.Millisecond
		})
		if got := startsWithLive(t, h, "proj:one-offs", "proj:one-offs/01", tc.live); got != tc.starts {
			t.Errorf("max-one-off-agents %d with %d live: started = %v, want %v", tc.oneOffCap, tc.live, got, tc.starts)
		}
	}
}

func TestIsOneOffEpic_ByNameAndLegacyFile(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/store/proj/one-offs", true},
		{"/store/proj/one-offs-archived", true},
		{"/store/proj/ordinary", false},
	} {
		if got := server.IsOneOffEpic(tickets.Epic{Path: tc.path}); got != tc.want {
			t.Errorf("IsOneOffEpic(%s) = %v, want %v", tc.path, got, tc.want)
		}
	}
	legacy := tickets.Epic{Path: "/store/proj/old", Tickets: []tickets.Ticket{{Path: "/store/proj/old/issues/01-one-off.md"}}}
	if !server.IsOneOffEpic(legacy) {
		t.Error("old single-ticket one-off epic is not detected")
	}
}

// An old single-ticket epic keeps the per-root cap; the one-off cap is for the
// one-offs epic only.
func TestRunner_OldOneOffEpicKeepsThePerRootCap(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicketWith(t, store, "proj", "legacy", "01", "one-off", servertest.TicketOpts{Type: "prompt"})
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.MaxAgentsPerRoot = 1
		c.MaxOneOffAgents = 3
		c.PollInterval = 50 * time.Millisecond
	})
	if startsWithLive(t, h, "proj:legacy", "proj:legacy/01", 1) {
		t.Error("old one-off epic started a second agent past the per-root cap")
	}
}
