package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

func startHeadlessRestart(t *testing.T, verdict nativerunner.Verdict, retention time.Duration) (*headlessFake, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := servertest.NewHeadlessFake(verdict)
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.LogRetention = retention
	})
	_, ticketPath := leaveIteration(t, h, store, repo, "herdr")
	asHeadlessRun(t, h)
	h.Restart(t)
	return fake, ticketPath
}

func TestReattach_DiedPausesStuckAndKeepsTheDir(t *testing.T) {
	t.Parallel()
	fake, ticketPath := startHeadlessRestart(t, nativerunner.VerdictDied, 0)
	servertest.WaitFor(t, 20*time.Second, "the ticket to pause for repair", func() bool {
		tk, err := schema.ParseTicket(ticketPath)
		if err != nil {
			t.Fatal(err)
		}
		return tk.Status == schema.StatusPaused && tk.PausedClass() == schema.PauseClassRepair
	})
	body, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(body), nativerunner.ReasonAgentDied) {
		t.Errorf("ticket lacks %q:\n%s", nativerunner.ReasonAgentDied, body)
	}
	if cleaned := fake.Cleaned(); len(cleaned) != 0 {
		t.Errorf("cleaned %v, want the dir kept", cleaned)
	}
}

func TestReattach_FinishedMergesAndCleansUp(t *testing.T) {
	t.Parallel()
	fake, ticketPath := startHeadlessRestart(t, nativerunner.VerdictFinished, 0)
	servertest.WaitFor(t, 30*time.Second, "the ticket to merge", func() bool {
		tk, err := schema.ParseTicket(ticketPath)
		if err != nil {
			t.Fatal(err)
		}
		return tk.Status == schema.StatusDone
	})
	var cleaned []string
	servertest.WaitFor(t, 30*time.Second, "the agent to be cleaned up", func() bool {
		cleaned = fake.Cleaned()
		return len(cleaned) > 0
	})
	if cleaned[0] != "epic-a-iter-01" {
		t.Errorf("cleaned = %v, want epic-a-iter-01", cleaned)
	}
}

func TestPrune_RunsAtStartWithConfiguredRetentionAndSkipsPaused(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	ticket := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	if err := ralphloop.Claim(ticket); err != nil {
		t.Fatal(err)
	}
	if err := ralphloop.Pause(filepath.Join(store, "proj"), "epic-a", "01", ticket, events.IterationError, "stuck", ralphloop.Event{}); err != nil {
		t.Fatal(err)
	}
	fake := servertest.NewHeadlessFake(0)
	servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.LogRetention = 36 * time.Hour
	})
	var paused map[string]bool
	var retention time.Duration
	servertest.WaitFor(t, 10*time.Second, "Prune to run", func() bool {
		paused, retention = fake.PruneSeen()
		return paused != nil
	})
	if retention != 36*time.Hour {
		t.Errorf("retention = %v, want 36h", retention)
	}
	if !paused["epic-a-iter-01"] || paused["epic-a-iter-02"] {
		t.Errorf("paused = %v, want only epic-a-iter-01", paused)
	}
}
