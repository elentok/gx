package server_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

// headlessFake is a fake runner with the headless runner's reattach, cleanup
// and prune hooks.
type headlessFake struct {
	*runnerfake.Runner
	verdict nativerunner.Verdict

	mu        sync.Mutex
	cleaned   []string
	stopped   []string
	retention time.Duration
	paused    map[string]bool
}

func (h *headlessFake) Stop(s agentrunner.Session) error {
	h.mu.Lock()
	h.stopped = append(h.stopped, s.ID)
	h.mu.Unlock()
	return h.Runner.Stop(s)
}

func (h *headlessFake) Reattach(label string) (agentrunner.Session, nativerunner.Verdict, error) {
	if s, ok, _ := h.Find(label); ok {
		return s, h.verdict, nil
	}
	return agentrunner.Session{Label: label, ID: "p1"}, h.verdict, nil
}

func (h *headlessFake) Cleanup(label string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleaned = append(h.cleaned, label)
	return nil
}

func (h *headlessFake) Prune(maxAge time.Duration, _ time.Time, paused func(string) bool) ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.retention = maxAge
	// Called once per project; only the proj call sees these labels.
	if h.paused == nil {
		h.paused = map[string]bool{}
	}
	for _, l := range []string{"epic-a-iter-01", "epic-a-iter-02"} {
		h.paused[l] = h.paused[l] || paused(l)
	}
	return nil, nil
}

// asHeadlessRun rewrites the staged handle as a headless run on the fake runner.
func asHeadlessRun(t *testing.T, h *servertest.Harness) {
	t.Helper()
	stampRun(t, h, "fake", agentrunner.Session{Label: "epic-a-iter-01", ID: "p1"})
}

// stampRun rewrites the staged handle's runner and session.
func stampRun(t *testing.T, h *servertest.Harness, runner string, session agentrunner.Session) {
	t.Helper()
	if runner == "" {
		t.Fatal("stampRun needs a runner")
	}
	path := filepath.Join(h.StateDir, "runs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs []map[string]any
	if err := json.Unmarshal(data, &runs); err != nil {
		t.Fatal(err)
	}
	runs[0]["runner"] = runner
	runs[0]["session"] = session
	delete(runs[0], "pane")
	if data, err = json.Marshal(runs); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func startHeadlessRestart(t *testing.T, verdict nativerunner.Verdict, retention time.Duration) (*headlessFake, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := &headlessFake{Runner: runnerfake.NewRunner(), verdict: verdict}
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
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.cleaned) != 0 {
		t.Errorf("cleaned %v, want the dir kept", fake.cleaned)
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
		fake.mu.Lock()
		defer fake.mu.Unlock()
		cleaned = slices.Clone(fake.cleaned)
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
	fake := &headlessFake{Runner: runnerfake.NewRunner()}
	servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.LogRetention = 36 * time.Hour
	})
	var paused map[string]bool
	var retention time.Duration
	servertest.WaitFor(t, 10*time.Second, "Prune to run", func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		paused, retention = fake.paused, fake.retention
		return paused != nil
	})
	if retention != 36*time.Hour {
		t.Errorf("retention = %v, want 36h", retention)
	}
	if !paused["epic-a-iter-01"] || paused["epic-a-iter-02"] {
		t.Errorf("paused = %v, want only epic-a-iter-01", paused)
	}
}
