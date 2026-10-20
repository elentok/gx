package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// A native Claude agent that asks and goes idle stays alive: the ticket parks
// self-reported, but the worktree, session and run remain, flagged waiting.
func TestAsk_NativeAgentStaysAliveWaiting(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	ticketPath := store + "/proj/epic-a/issues/01-first.md"
	var worktree string
	fake := newTurnRunner(func(_, cwd string) {
		worktree = cwd
		reportIteration(t, ticketPath, "needs-answer")
	})
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.RunnerFor = func(_, _ string) agentrunner.Runner { return fake }
	})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	waitStatus(t, ticketPath, schema.StatusNeedsAnswer)
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.ParkKind != "self-reported" {
		t.Errorf("park_kind = %q, want self-reported", tk.ParkKind)
	}
	waitFor(t, func() bool {
		data, _ := os.ReadFile(filepath.Join(h.StateDir, "runs.json"))
		return strings.Contains(string(data), `"waiting_since"`)
	})
	if _, err := os.Stat(worktree); err != nil {
		t.Errorf("worktree gone: %v", err)
	}
	if _, ok, err := fake.Find("epic-a-iter-01"); err != nil || !ok {
		t.Errorf("session not live: ok=%v err=%v", ok, err)
	}
	if runs := h.Server.Runs(); len(runs) != 1 {
		t.Errorf("Runs() = %+v, want the waiting run kept", runs)
	}
	if body, _ := os.ReadFile(ticketPath); strings.Contains(string(body), "waiting") {
		t.Errorf("ticket records waiting state:\n%s", body)
	}
}
