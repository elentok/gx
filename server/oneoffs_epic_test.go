package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
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

// waitForAgentStarts returns the --kind of each started agent, by the agent
// name herdr was given (one-offs-iter-<id>), once want agents have started.
func waitForAgentStarts(t *testing.T, h *servertest.Harness, want int) map[string]string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		kinds := map[string]string{}
		for _, e := range h.Herdr.Trace() {
			if len(e.Argv) > 3 && e.Argv[0] == "agent" && e.Argv[1] == "start" {
				for i, a := range e.Argv {
					if a == "--kind" && i+1 < len(e.Argv) {
						kinds[e.Argv[2]] = e.Argv[i+1]
					}
				}
			}
		}
		if len(kinds) >= want {
			return kinds
		}
		if time.Now().After(deadline) {
			t.Fatalf("agent starts = %v, want %d", kinds, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func startOneOffsHarness(t *testing.T, store string) *servertest.Harness {
	t.Helper()
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	return servertest.StartWithStore(t, store, func(c *server.Config) {
		c.MaxAgentsPerRoot = 1
		c.MaxOneOffAgents = 2
		c.MaxAgents = 10
		c.PollInterval = 50 * time.Millisecond
	})
}

// A claude one-off and a codex one-off run side by side under one cap, each
// under its own ticket's agent, whatever agent the queue items name.
func TestRunner_OneOffsRunClaudeAndCodexUnderOneCap(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Agent: "claude"})
	servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, "02", "b", servertest.TicketOpts{Type: "prompt", Agent: "codex"})
	h := startOneOffsHarness(t, store)
	h.RegisterLaunch(func(servertest.Prompt) {})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, addr := range []string{"proj:one-offs/01", "proj:one-offs/02"} {
		if res, err := h.Client.QueueAdd(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	got := waitForAgentStarts(t, h, 2)
	if got["one-offs-iter-01"] != "claude" || got["one-offs-iter-02"] != "codex" {
		t.Errorf("agent kinds = %v, want 01 claude and 02 codex", got)
	}
}

// A one-off with no agent field, or an unknown one, launches with the agent of
// its queue item.
func TestRunner_OneOffWithoutAKnownAgentUsesTheQueueItemAgent(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt"})
	servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, "02", "b", servertest.TicketOpts{Type: "prompt", Agent: "nonsense"})
	h := startOneOffsHarness(t, store)
	h.RegisterLaunch(func(servertest.Prompt) {})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, addr := range []string{"proj:one-offs/01", "proj:one-offs/02"} {
		if res, err := h.Client.QueueAdd(ctx, addr, "codex"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	for name, kind := range waitForAgentStarts(t, h, 2) {
		if kind != "codex" {
			t.Errorf("%s started as %q, want the queue item's codex", name, kind)
		}
	}
}

func TestTicketAgent(t *testing.T) {
	for _, tc := range []struct{ field, fallback, want string }{
		{"", "claude", "claude"},
		{"nonsense", "codex", "codex"},
		{"codex", "claude", "codex"},
		{"claude", "codex", "claude"},
	} {
		got := server.TicketAgent(tickets.Ticket{Agent: tc.field}, ralphloop.AgentKind(tc.fallback))
		if string(got) != tc.want {
			t.Errorf("ticketAgent(field %q, fallback %q) = %q, want %q", tc.field, tc.fallback, got, tc.want)
		}
	}
}

// The one-offs epic is a standing inbox: with every ticket done it is not
// stamped, merged or retro'd.
func TestCompleteRootIfDone_AllDoneOneOffsEpicNeverLands(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Status: "done"})
	h := startOneOffsHarness(t, store)
	start, _, _ := registerLaunch(h)
	merged := false
	h.Server.SetMergeInto(func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error) {
		merged = true
		return git.MergeOutcome{}, nil
	})
	epicMD := filepath.Join(store, "proj", server.OneOffsEpic, "ticket.md")
	before, err := os.ReadFile(epicMD)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Server.CompleteRootIfDone("proj:one-offs/01"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(epicMD)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("one-offs epic ticket.md changed:\n%s", after)
	}
	if merged {
		t.Error("one-offs epic was merged")
	}
	if len(*start) != 0 {
		t.Errorf("an agent started (retro?): %v", *start)
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
