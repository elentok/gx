package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// herdrNamed is a turnRunner that answers to "herdr", so its runs are herdr-hosted.
type herdrNamed struct{ *turnRunner }

func (herdrNamed) Name() string { return "herdr" }

// codexNamed is a turnRunner on a native runner, hosting a Codex agent.
type codexNamed struct{ *turnRunner }

func (codexNamed) Name() string { return "headless" }

// An ask from a herdr-hosted Claude or a Codex run cannot be answered live:
// the ticket pauses, the session closes and the run leaves the registry.
func TestAsk_HerdrClaudeAndCodexCloseSession(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, agent string
		wrap        func(*turnRunner) agentrunner.Runner
	}{
		{"herdr claude", "claude", func(r *turnRunner) agentrunner.Runner { return herdrNamed{r} }},
		{"native codex", "codex", func(r *turnRunner) agentrunner.Runner { return codexNamed{r} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
			servertest.SetProjectRepo(t, store, "proj", repo)
			ticketPath := store + "/proj/epic-a/issues/01-first.md"
			turn := newTurnRunner(func(_, _ string) { reportIteration(t, ticketPath, "asking") })
			fake := c.wrap(turn)
			h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
			if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", c.agent); err != nil || res.Refused {
				t.Fatalf("add: %+v, %v", res, err)
			}
			waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
			waitFor(t, func() bool { return len(h.Server.Runs()) == 0 })
			if _, ok, err := fake.Find("epic-a-iter-01"); err != nil || ok {
				t.Errorf("session still live: ok=%v err=%v", ok, err)
			}
			if data, _ := os.ReadFile(filepath.Join(h.StateDir, "runs.json")); strings.Contains(string(data), `"waiting_since"`) {
				t.Errorf("run marked waiting:\n%s", data)
			}
		})
	}
}

// A native Claude agent that asks and goes idle stays alive: the ticket pauses
// self-reported, but the worktree, session and run remain, flagged waiting.
func TestAsk_NativeAgentStaysAliveWaiting(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	ticketPath := store + "/proj/epic-a/issues/01-first.md"
	var worktree string
	fake := newTurnRunner(func(_, cwd string) {
		worktree = cwd
		reportIteration(t, ticketPath, "asking")
	})
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	assertPaused(t, tk, schema.PauseClassAnswer, schema.PauseSelfReported)
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
