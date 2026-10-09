package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/testutil/runnerfake"
)

func TestBudget_SoftLimitStopsNewStarts(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.BudgetSoftLimit = 5
	})
	registerLaunch(h)
	h.Server.RecordSpend("spent", 6)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	time.Sleep(500 * time.Millisecond)
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Fatalf("runs = %+v, want none past the soft limit", runs)
	}
}

func TestBudget_SpendInTwoProjectsSumsTowardOneLimit(t *testing.T) {
	store, repoA, repoB := t.TempDir(), testutil.TempRepo(t), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj-a", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj-b", "epic-b", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj-a", repoA)
	servertest.SetProjectRepo(t, store, "proj-b", repoB)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.BudgetSoftLimit = 5
	})
	registerLaunch(h)
	h.Server.RecordProjectSpend("proj-a", "a", 3)
	h.Server.RecordProjectSpend("proj-b", "b", 3)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	status, err := h.Client.Budget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Total != 6 || !status.BudgetPaused {
		t.Errorf("status = %+v, want total 6 and paused", status)
	}
	if status.Projects["proj-a"] != 3 || status.Projects["proj-b"] != 3 {
		t.Errorf("projects = %v, want 3 each", status.Projects)
	}
	for _, addr := range []string{"proj-a:epic-a/01", "proj-b:epic-b/01"} {
		if res, err := h.Client.QueueAdd(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	time.Sleep(500 * time.Millisecond)
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Fatalf("runs = %+v, want none in either project past the soft limit", runs)
	}
}

func TestBudget_HardLimitStopsLivePaneAndParksBudgetKilled(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.BudgetHardLimit = 5
		c.BudgetKillGrace = time.Millisecond
		c.BudgetPollInterval = time.Hour
	})
	registerLaunch(h)
	live := map[string]any{"agent": map[string]any{
		"pane_id": "p1", "tab_id": "t1", "agent_status": "working",
		"agent_session": map[string]any{"value": "sess"},
	}}
	// The agent is idle until prompted (Start waits for idle), then working.
	idle := map[string]any{"agent": map[string]any{"pane_id": "p1", "tab_id": "t1", "agent_status": "idle"}}
	var started, prompted atomic.Bool
	h.Herdr.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		started.Store(true)
		return live, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "get", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		if !started.Load() {
			return nil, herdrfake.Identities{}, servertest.AgentNotFound(argv[2])
		}
		if !prompted.Load() {
			return idle, herdrfake.Identities{}, nil
		}
		return live, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "prompt", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		prompted.Store(true)
		return map[string]any{"agent": map[string]any{
			"pane_id": "p1", "tab_id": "t1", "agent_status": "working", "state_change_seq": 1,
			"agent_session": map[string]any{"value": "sess"},
		}}, herdrfake.Identities{}, nil
	})
	var sent, closed []string
	h.Herdr.Register("agent", "send-keys", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		sent = append(sent, strings.Join(argv, " "))
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "close", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		closed = append(closed, strings.Join(argv, " "))
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	// Cost rises on every read, so the pane never goes quiet after ctrl+c.
	cost := 0.0
	h.Server.SetCostOf(func(server.IterationInfo) (float64, bool) { cost += 3; return cost, true })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil {
		t.Fatal(err)
	}
	for len(h.Server.Runs()) == 0 && ctx.Err() == nil {
		time.Sleep(20 * time.Millisecond)
	}
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	for ctx.Err() == nil {
		h.Server.PollBudget()
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "budget-killed") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "status: needs-repair") || !strings.Contains(string(data), "budget-killed") {
		t.Fatalf("ticket not parked budget-killed:\n%s", data)
	}
	if len(sent) == 0 || !strings.Contains(sent[0], "ctrl+c") {
		t.Errorf("send-keys = %v, want ctrl+c", sent)
	}
	if len(closed) == 0 {
		t.Error("pane was not closed")
	}
}

func TestBudget_HardLimitStopsRunThroughRunner(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Runner = fake
		c.BudgetHardLimit = 5
		c.BudgetKillGrace = time.Millisecond
		c.BudgetPollInterval = time.Hour
	})
	cost := 0.0
	h.Server.SetCostOf(func(server.IterationInfo) (float64, bool) { cost += 3; return cost, true })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil {
		t.Fatal(err)
	}
	for len(h.Server.Runs()) == 0 && ctx.Err() == nil {
		time.Sleep(20 * time.Millisecond)
	}
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	for ctx.Err() == nil {
		h.Server.PollBudget()
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "budget-killed") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "budget-killed") {
		t.Fatalf("ticket not parked budget-killed:\n%s", data)
	}
	if live, _ := fake.List("epic-a"); len(live) != 0 {
		t.Errorf("sessions still live after budget kill: %v", live)
	}
}
