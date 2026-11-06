package sched_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

func mustFindRepo(t *testing.T, dir string) *git.Repo {
	t.Helper()
	repo, err := git.FindRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

// A launch that fails with a typed cause reaches Chat as one plain sentence,
// while the ticket file keeps the full reason.
func TestLaunchFailure_TypedCausesReachChatAsSentences(t *testing.T) {
	t.Parallel()
	taken := t.TempDir() // git only refuses a folder that is not empty
	if err := os.WriteFile(filepath.Join(taken, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	existsErr := git.AddWorktree(*mustFindRepo(t, testutil.TempRepo(t)), "taken", taken, "HEAD")
	cases := []struct {
		name     string
		err      error
		sentence string
	}{
		{"worktree exists", existsErr, "The ticket's worktree folder already exists."},
		{"session died", agentrunner.SessionDied("holder socket /tmp/gx-holder.sock timeout"), "The agent session ended unexpectedly."},
		{"herdr lost", herdr.NewAgentNotFoundError("agent_not_found", errors.New("herdr agent get: agent_not_found")), "herdr lost track of the agent."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
			servertest.SetProjectRepo(t, store, "proj", repo)
			chat, count := chatRecorder(t)
			h := servertest.StartWithStore(t, store, chat)
			h.FailNextStart(c.err)
			if res, err := h.Client.Queue.Add(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
				t.Fatalf("add: %+v, %v", res, err)
			}
			if got := waitForChat(count, c.sentence); got != 1 {
				t.Errorf("sentence %q sent %d times, want once", c.sentence, got)
			}
			for _, leak := range []string{"gx-holder.sock", "agent_not_found", "stall-nudge", "session-died", "herdr-agent-lost"} {
				if count(leak) != 0 {
					t.Errorf("chat leaks %q", leak)
				}
			}
			path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
			servertest.WaitFor(t, 10*time.Second, "the ticket to pause", func() bool {
				tk, err := schema.ParseTicket(path)
				return err == nil && tk.Status == schema.StatusPaused
			})
			body, _ := os.ReadFile(path)
			if !strings.Contains(string(body), "launch failed") || !strings.Contains(string(body), c.err.Error()) {
				t.Errorf("ticket lost the full reason:\n%s", body)
			}
		})
	}
}

// A holder that died while the server was down is worded plainly in Chat; the
// ticket keeps the constant reason (TestReattach_DiedPausesStuckAndKeepsTheDir).
func TestReclaim_DiedHolderReachesChatAsASentence(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := servertest.NewHeadlessFake(nativerunner.VerdictDied)
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), chat)
	leaveIteration(t, h, store, repo, "herdr")
	asHeadlessRun(t, h)
	h.Restart(t)
	if got := waitForChat(count, "The agent session ended unexpectedly."); got != 1 {
		t.Errorf("sentence sent %d times, want once", got)
	}
	if n := count(nativerunner.ReasonAgentDied); n != 0 {
		t.Errorf("chat leaks %q", nativerunner.ReasonAgentDied)
	}
}
