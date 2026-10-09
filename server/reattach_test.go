package server_test

import (
	"context"
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
	retention time.Duration
	parked    map[string]bool
}

func (h *headlessFake) Reattach(label string) (agentrunner.Session, nativerunner.Verdict, error) {
	return agentrunner.Session{Label: label, ID: "p1"}, h.verdict, nil
}

func (h *headlessFake) Cleanup(label string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleaned = append(h.cleaned, label)
	return nil
}

func (h *headlessFake) Prune(maxAge time.Duration, _ time.Time, parked func(string) bool) ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.retention = maxAge
	// Called once per project; only the proj call sees these labels.
	if h.parked == nil {
		h.parked = map[string]bool{}
	}
	for _, l := range []string{"epic-a-iter-01", "epic-a-iter-02"} {
		h.parked[l] = h.parked[l] || parked(l)
	}
	return nil, nil
}

// asHeadlessRun rewrites the staged handle as a headless run on the fake runner.
func asHeadlessRun(t *testing.T, h *servertest.Harness) {
	t.Helper()
	path := filepath.Join(h.StateDir, "runs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs []map[string]any
	if err := json.Unmarshal(data, &runs); err != nil {
		t.Fatal(err)
	}
	runs[0]["runner"] = "fake"
	runs[0]["session"] = agentrunner.Session{Label: "epic-a-iter-01", ID: "p1"}
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
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.RunnerName = "fake"
		c.RunnerFor = func(string) agentrunner.Runner { return fake }
		c.LogRetention = retention
	})
	_, ticketPath := leaveIteration(t, h, store, repo)
	asHeadlessRun(t, h)
	h.Restart(t)
	return fake, ticketPath
}

func TestReattach_DiedParksNeedsRepairAndKeepsTheDir(t *testing.T) {
	fake, ticketPath := startHeadlessRestart(t, nativerunner.VerdictDied, 0)
	deadline := time.Now().Add(20 * time.Second)
	for {
		tk, err := schema.ParseTicket(ticketPath)
		if err != nil {
			t.Fatal(err)
		}
		if tk.Status == schema.StatusNeedsRepair {
			body, _ := os.ReadFile(ticketPath)
			if !strings.Contains(string(body), nativerunner.ReasonAgentDied) {
				t.Errorf("ticket lacks %q:\n%s", nativerunner.ReasonAgentDied, body)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("status = %q, want needs-repair", tk.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.cleaned) != 0 {
		t.Errorf("cleaned %v, want the dir kept", fake.cleaned)
	}
}

func TestReattach_FinishedLandsAndCleansUp(t *testing.T) {
	fake, ticketPath := startHeadlessRestart(t, nativerunner.VerdictFinished, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		tk, err := schema.ParseTicket(ticketPath)
		if err != nil {
			t.Fatal(err)
		}
		if tk.Status == schema.StatusDone {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal("ticket never landed")
	}
	for ; ctx.Err() == nil; time.Sleep(50 * time.Millisecond) {
		fake.mu.Lock()
		cleaned := slices.Clone(fake.cleaned)
		fake.mu.Unlock()
		if len(cleaned) > 0 {
			if cleaned[0] != "epic-a-iter-01" {
				t.Errorf("cleaned = %v, want epic-a-iter-01", cleaned)
			}
			return
		}
	}
	t.Error("the agent was never cleaned up")
}

func TestPrune_RunsAtStartWithConfiguredRetentionAndSkipsParked(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	ticket := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	if err := ralphloop.Claim(ticket); err != nil {
		t.Fatal(err)
	}
	if err := ralphloop.Park(filepath.Join(store, "proj"), "epic-a", "01", ticket, events.IterationError, "stuck"); err != nil {
		t.Fatal(err)
	}
	fake := &headlessFake{Runner: runnerfake.NewRunner()}
	servertest.StartWithStore(t, store, func(c *server.Config) {
		c.RunnerFor = func(string) agentrunner.Runner { return fake }
		c.LogRetention = 36 * time.Hour
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		fake.mu.Lock()
		parked, retention := fake.parked, fake.retention
		fake.mu.Unlock()
		if parked != nil {
			if retention != 36*time.Hour {
				t.Errorf("retention = %v, want 36h", retention)
			}
			if !parked["epic-a-iter-01"] || parked["epic-a-iter-02"] {
				t.Errorf("parked = %v, want only epic-a-iter-01", parked)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Prune never ran")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
