package server_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

// unnamedRunner is the fake with an empty runner name, which keepsAskedAgent
// treats as non-keeping (the same as herdr).
type unnamedRunner struct{ *runnerfake.Runner }

func (unnamedRunner) Name() string { return "" }

// A non-keeping runner drops its agent session on an
// ask but keeps the iteration worktree, so the relaunch finds the agent's
// uncommitted work. The pause holds no permit: a second ticket claims.
func TestAsk_NonKeepingRunnerKeepsTheWorktreeForTheRelaunch(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(unnamedRunner{fake}), func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
		c.MaxAgentsPerRoot = 1
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	const label = "epic-a-iter-01"
	t.Cleanup(func() { fake.StopAll() })

	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	// The launch prompt flips the fake to working; end the turn only after it.
	waitFor(t, func() bool { return len(fake.Prompts(label)) > 0 })
	wt := ""
	for _, line := range strings.Split(git("worktree", "list", "--porcelain"), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok && strings.Contains(p, "item-01") {
			wt = p
		}
	}
	if wt == "" {
		t.Fatalf("no iteration worktree:\n%s", git("worktree", "list"))
	}
	testutil.WriteFile(t, wt, "wip.txt", "uncommitted")

	ticketPath := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	if res, err := h.Client.TicketAsk(ctx, "proj:epic-a/01", askBody); err != nil || res.Refused {
		t.Fatalf("ask = %+v, %v", res, err)
	}
	fake.SetState(label, agentrunner.StateIdle, "")

	// The ask releases the session, and the second ticket claims the freed permit.
	waitFor(t, func() bool { _, ok, _ := fake.Find(label); return !ok })
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/02", "claude"); err != nil || res.Refused {
		t.Fatalf("add 02: %+v, %v", res, err)
	}
	waitFor(t, func() bool { _, ok, _ := fake.Find("epic-a-iter-02"); return ok })
	if _, err := os.Stat(filepath.Join(wt, "wip.txt")); err != nil {
		t.Fatalf("uncommitted file lost after the ask: %v", err)
	}
	if got := strings.TrimSpace(git("branch", "--list", "ralph-loop/epic-a-item-01")); got == "" {
		t.Error("iteration branch gone after the ask")
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	assertPaused(t, tk, schema.PauseClassAnswer, "")

	// Answer, then relaunch: the worktree is reused as it is.
	// End 02's turn (re-asserted: its launch prompt flips the fake back to working).
	waitFor(t, func() bool {
		fake.SetState("epic-a-iter-02", agentrunner.StateIdle, "")
		return len(h.Server.Runs()) == 0
	})
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	if res, err := h.Client.TicketAnswer(ctx, server.QueueRequest{Address: answerAddr, State: state, Choice: "a"}); err != nil || res.Refused {
		t.Fatalf("answer = %+v, %v", res, err)
	}
	waitFor(t, func() bool { _, ok, _ := fake.Find(label); return ok })
	if data, err := os.ReadFile(filepath.Join(wt, "wip.txt")); err != nil || string(data) != "uncommitted" {
		t.Errorf("wip.txt after relaunch = %q, %v; want it kept", data, err)
	}
	waitFor(t, func() bool { return len(fake.Prompts(label)) > 0 })
	for _, p := range fake.Prompts(label) {
		if strings.Contains(strings.ToLower(p), "uncommitted") {
			t.Errorf("launch prompt mentions uncommitted work: %q", p)
		}
	}
}

// A commitless ticket on a non-keeping runner keeps its detached worktree when
// it asks, the same as an implement ticket does.
func TestAsk_CommitlessOnNonKeepingRunnerKeepsTheWorktreeForTheRelaunch(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Type: "investigate"})
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(unnamedRunner{fake}), func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const label = "epic-a-iter-01"
	t.Cleanup(func() { fake.StopAll() })

	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	waitFor(t, func() bool { return len(fake.Prompts(label)) > 0 })
	list, err := exec.Command("git", "-C", repo, "worktree", "list", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	wt := ""
	for _, line := range strings.Split(string(list), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok && strings.Contains(p, "item-01") {
			wt = p
		}
	}
	if wt == "" {
		t.Fatalf("no iteration worktree:\n%s", list)
	}
	testutil.WriteFile(t, wt, "wip.txt", "uncommitted")

	ticketPath := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	if res, err := h.Client.TicketAsk(ctx, "proj:epic-a/01", askBody); err != nil || res.Refused {
		t.Fatalf("ask = %+v, %v", res, err)
	}
	fake.SetState(label, agentrunner.StateIdle, "")
	waitFor(t, func() bool { _, ok, _ := fake.Find(label); return !ok })
	if _, err := os.Stat(filepath.Join(wt, "wip.txt")); err != nil {
		t.Fatalf("uncommitted file lost after the ask: %v", err)
	}

	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	if res, err := h.Client.TicketAnswer(ctx, server.QueueRequest{Address: answerAddr, State: state, Choice: "a"}); err != nil || res.Refused {
		t.Fatalf("answer = %+v, %v", res, err)
	}
	waitFor(t, func() bool { _, ok, _ := fake.Find(label); return ok })
	if data, err := os.ReadFile(filepath.Join(wt, "wip.txt")); err != nil || string(data) != "uncommitted" {
		t.Errorf("wip.txt after relaunch = %q, %v; want it kept", data, err)
	}
	waitFor(t, func() bool { return len(fake.Prompts(label)) > 0 })
	for _, p := range fake.Prompts(label) {
		if strings.Contains(strings.ToLower(p), "uncommitted") {
			t.Errorf("launch prompt mentions uncommitted work: %q", p)
		}
	}
}
