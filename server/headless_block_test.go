package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// startBlockable starts proj:epic-a/01 on a runner named runnerName whose agent
// keeps working until the test blocks it, and returns once the session is live.
func startBlockable(t *testing.T, runnerName string) (*servertest.Harness, string, *turnRunner) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	ticketPath := store + "/proj/epic-a/issues/01-first.md"
	turn := newTurnRunner(func(_, _ string) {})
	turn.PromptState = agentrunner.StateWorking
	var fake agentrunner.Runner = codexNamed{turn}
	if runnerName == "herdr" {
		fake = herdrNamed{turn}
	}
	h := servertest.StartWithStore(t, store, func(cfg *server.Config) {
		cfg.Runner = fake
		cfg.RunnerFor = func(_, _ string) agentrunner.Runner { return fake }
	})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	waitFor(t, func() bool { _, ok, _ := turn.Find("epic-a-iter-01"); return ok })
	return h, ticketPath, turn
}

func TestHeadlessBlock_PausesAtOnce(t *testing.T) {
	cases := []struct {
		name  string
		block func(*turnRunner)
		want  []string
	}{
		{"permission request", func(r *turnRunner) {
			r.SetPending("epic-a-iter-01", &agentrunner.Pending{ID: "r1", Tool: "Bash", Input: []byte(`{"command":"ls"}`)})
		}, []string{"Bash", `"command":"ls"`, "gx server agents answer"}},
		{"bare requires_action", func(r *turnRunner) {
			r.SetState("epic-a-iter-01", agentrunner.StateBlocked, "waiting on a prompt")
		}, []string{"waiting on a prompt", "gx server agents interrupt"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, ticketPath, turn := startBlockable(t, "headless")
			c.block(turn)
			waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
			tk, err := schema.ParseTicket(ticketPath)
			if err != nil {
				t.Fatal(err)
			}
			assertPaused(t, tk, schema.PauseClassAnswer, schema.PauseBlockedPane)
			// The pause hands the run to the waiting-run machinery.
			waitFor(t, func() bool {
				data, _ := os.ReadFile(filepath.Join(h.StateDir, "runs.json"))
				return strings.Contains(string(data), `"waiting_since"`)
			})
			if _, ok, err := turn.Find("epic-a-iter-01"); err != nil || !ok {
				t.Errorf("session not live: ok=%v err=%v", ok, err)
			}
			var reason string
			for _, ev := range runLog(t, h) {
				if ev.Type == "paused" && ev.Kind == "blocked-pane" {
					reason = ev.Reason
				}
			}
			for _, w := range c.want {
				if !strings.Contains(reason, w) {
					t.Errorf("pause reason %q lacks %q", reason, w)
				}
			}
		})
	}
}

// A herdr-hosted agent that blocks keeps the dwell path: the ticket stays claimed.
func TestHeadlessBlock_HerdrStaysClaimed(t *testing.T) {
	_, ticketPath, turn := startBlockable(t, "herdr")
	turn.SetState("epic-a-iter-01", agentrunner.StateBlocked, "waiting on a prompt")
	time.Sleep(500 * time.Millisecond)
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusClaimed {
		t.Errorf("status = %q, want claimed", tk.Status)
	}
}
