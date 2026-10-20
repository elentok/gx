package server_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Relaunch and nudge start agent work outside the queue (recovery remedies use
// them too), so the soft limit refuses them the way it holds claims back.
func TestBudget_SoftLimitRefusesRelaunchAndNudge(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.BudgetSoftLimit = 5
	})
	start, _, _ := registerLaunch(h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	h.Server.RecordSpend("spent", 6)

	if res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01", false); err != nil || !res.Refused || res.Reason != server.ReasonBudgetLimit {
		t.Errorf("relaunch = %+v, %v; want refusal %s", res, err, server.ReasonBudgetLimit)
	}
	if len(*start) != 0 {
		t.Errorf("agent started past the soft limit: %v", *start)
	}
	if res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "finish up"); err != nil || !res.Refused || res.Reason != server.ReasonBudgetLimit {
		t.Errorf("nudge = %+v, %v; want refusal %s", res, err, server.ReasonBudgetLimit)
	}
}

// A nudge into a parked ticket's still-open pane (recovery's R7 does this)
// sets an agent working outside any run; it must be metered and the hard
// limit must stop it like any other live agent.
func TestBudget_NudgedParkedPaneIsMeteredAndKilled(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.BudgetHardLimit = 5
		c.BudgetKillGrace = 50 * time.Millisecond
		c.BudgetPollInterval = time.Hour
	})
	if err := h.Server.ParkAs("proj:epic-a/01", events.IterationError, "compaction timed out"); err != nil {
		t.Fatal(err)
	}
	var gone, closed atomic.Bool
	t.Cleanup(func() { gone.Store(true) }) // lets the pane's watch end
	working := map[string]any{"agent": map[string]any{
		"pane_id": "p1", "tab_id": "t1", "agent_status": "working",
		"agent_session": map[string]any{"value": "sess"},
	}}
	idle := map[string]any{"agent": map[string]any{
		"pane_id": "p1", "tab_id": "t1", "agent_status": "idle",
		"agent_session": map[string]any{"value": "sess"},
	}}
	var nudged atomic.Bool
	live := func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		if gone.Load() {
			return nil, herdrfake.Identities{}, servertest.AgentNotFound(argv[2])
		}
		return working, herdrfake.Identities{}, nil
	}
	// The pane idles until nudged: a prompt to a working pane is refused.
	h.Herdr.Register("agent", "get", func(s *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		if !nudged.Load() && !gone.Load() {
			return idle, herdrfake.Identities{}, nil
		}
		return live(s, argv)
	})
	h.Herdr.Register("agent", "wait", live)
	h.Herdr.Register("agent", "prompt", func(s *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		nudged.Store(true)
		return live(s, argv)
	})
	h.Herdr.Register("agent", "send-keys", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		closed.Store(true)
		gone.Store(true)
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	var mu sync.Mutex
	cost := 0.0
	h.Server.SetCostOf(func(server.IterationInfo) (float64, bool) { mu.Lock(); defer mu.Unlock(); cost += 3; return cost, true })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "finish up"); err != nil || res.Refused {
		t.Fatalf("nudge = %+v, %v", res, err)
	}
	if runs := h.Server.Runs(); len(runs) != 1 {
		t.Fatalf("runs = %+v, want the nudged pane tracked", runs)
	}
	for len(h.Server.Runs()) != 0 && ctx.Err() == nil {
		h.Server.PollBudget()
		time.Sleep(20 * time.Millisecond)
	}
	if !closed.Load() {
		t.Error("the hard limit did not close the nudged pane")
	}
	tk, err := schema.ParseTicket(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(tk.ParkKind) != string(events.BudgetKilled) {
		t.Errorf("park kind = %s, want budget-killed", tk.ParkKind)
	}
}

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
	// The pane never goes quiet, so the run's finish wait would poll herdr
	// until the package ends; take the agent away so the run settles first.
	t.Cleanup(func() {
		h.Herdr.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
			return nil, herdrfake.Identities{}, servertest.AgentNotFound(argv[2])
		})
	})
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

// An agent that keeps spending after the ctrl+c is closed and parked
// budget-killed, and its run's own finish must not overwrite that park.
func TestBudget_HardLimitStopsRunThroughRunner(t *testing.T) {
	var mu sync.Mutex
	cost := 0.0
	testBudgetKillParksBudgetKilled(t, func() float64 { mu.Lock(); defer mu.Unlock(); cost += 3; return cost }, servertest.TicketOpts{})
}

// An agent that goes quiet on the ctrl+c is parked budget-killed too, not
// left to its own finish (which would park it zero-commit, a kind recovery
// relaunches past the hard limit).
func TestBudget_HardLimitParksAQuietRunBudgetKilledToo(t *testing.T) {
	testBudgetKillParksBudgetKilled(t, func() float64 { return 6 }, servertest.TicketOpts{})
}

func testBudgetKillParksBudgetKilled(t *testing.T, cost func() float64, ticket servertest.TicketOpts) {
	t.Helper()
	runBudgetKill(t, cost, ticket, nil)
}

// A hard-limit kill closes the run's open question with budget-killed, before
// the park status.
func TestBudget_HardLimitKillClosesTheOpenQuestion(t *testing.T) {
	path := runBudgetKill(t, func() float64 { return 6 }, servertest.TicketOpts{}, func(path string) {
		if _, err := tickets.AppendLog(path, tickets.LogEvent{Type: tickets.EventParkQuestion, By: "gx", Summary: "q?"}); err != nil {
			t.Fatal(err)
		}
	})
	evs, err := tickets.ReadLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[1].Type != tickets.EventParkClosed || evs[1].Question != 1 || evs[1].Reason != "budget-killed" {
		t.Errorf("log = %+v, want the question then park.closed{question 1, budget-killed}", evs)
	}
}

func TestBudget_HardLimitKillWithoutQuestionWritesNoLog(t *testing.T) {
	path := runBudgetKill(t, func() float64 { return 6 }, servertest.TicketOpts{}, nil)
	if evs, _ := tickets.ReadLog(path); len(evs) != 0 {
		t.Errorf("log = %+v, want empty", evs)
	}
}

// runBudgetKill runs one ticket past the hard limit, seeding its log first
// when seed is set, and returns the ticket's path once it is parked.
func runBudgetKill(t *testing.T, cost func() float64, ticket servertest.TicketOpts, seed func(path string)) string {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", ticket)
	if seed != nil {
		seed(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Runner = fake
		c.BudgetHardLimit = 5
		c.BudgetKillGrace = 50 * time.Millisecond
		c.BudgetPollInterval = time.Hour
	})
	h.Server.SetCostOf(func(server.IterationInfo) (float64, bool) { return cost(), true })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil {
		t.Fatal(err)
	}
	for len(h.Server.Runs()) == 0 && ctx.Err() == nil {
		time.Sleep(20 * time.Millisecond)
	}
	// Poll until the run has settled, and judge the ticket only then: the
	// run's own finish is what used to overwrite the budget park.
	for len(h.Server.Runs()) != 0 && ctx.Err() == nil {
		h.Server.PollBudget()
		time.Sleep(20 * time.Millisecond)
	}
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	tk, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusNeedsRepair || string(tk.ParkKind) != string(events.BudgetKilled) {
		data, _ := os.ReadFile(path)
		t.Fatalf("ticket = %s/%s, want needs-repair/budget-killed:\n%s", tk.Status, tk.ParkKind, data)
	}
	if live, _ := fake.List("epic-a"); len(live) != 0 {
		t.Errorf("sessions still live after budget kill: %v", live)
	}
	return path
}

// startBudgetRuns launches n runs of epic-a, one fake session each, with the
// hard limit not yet reached. It returns the harness, the fake and the labels.
func startBudgetRuns(t *testing.T, n int, grace time.Duration) (*servertest.Harness, *runnerfake.Runner, *atomic.Bool, []string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	var labels []string
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("%02d", i)
		servertest.WriteTicket(t, store, "proj", "epic-a", id, "t"+id, "")
		labels = append(labels, "epic-a-iter-"+id)
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Runner = fake
		c.BudgetHardLimit = 5
		c.BudgetKillGrace = grace
		c.BudgetPollInterval = time.Hour
		c.MaxAgentsPerRoot = n
		c.MaxAgents = n
	})
	var over atomic.Bool
	h.Server.SetCostOf(func(server.IterationInfo) (float64, bool) {
		if over.Load() {
			return 6, true
		}
		return 0, true
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for i := 1; i <= n; i++ {
		if _, err := h.Client.QueueAdd(ctx, fmt.Sprintf("proj:epic-a/%02d", i), "claude"); err != nil {
			t.Fatal(err)
		}
	}
	for ctx.Err() == nil {
		if live, _ := fake.List("epic-a"); len(live) >= n {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if live, _ := fake.List("epic-a"); len(live) < n {
		t.Fatalf("live sessions = %d, want %d", len(live), n)
	}
	return h, fake, &over, labels
}

// The grace counts from the stop decision and the Interrupts run side by side:
// stopping N slow runs takes about the grace, not the sum of their Interrupts.
func TestBudget_HardLimitInterruptsRunInParallelInsideGrace(t *testing.T) {
	h, fake, over, labels := startBudgetRuns(t, 3, 400*time.Millisecond)
	for _, l := range labels {
		fake.SetInterruptDelay(l, 300*time.Millisecond)
	}
	over.Store(true)
	start := time.Now()
	h.Server.PollBudget()
	elapsed := time.Since(start)
	if elapsed < 400*time.Millisecond || elapsed >= 800*time.Millisecond {
		t.Errorf("stop took %v, want about the 400ms grace (serial Interrupts would take 900ms)", elapsed)
	}
	assertNoLiveSessions(t, fake)
}

// A grace shorter than the Interrupts does not cut them off: Stop waits for
// every Interrupt to return.
func TestBudget_HardLimitStopWaitsForSlowInterrupts(t *testing.T) {
	h, fake, over, labels := startBudgetRuns(t, 2, 20*time.Millisecond)
	for _, l := range labels {
		fake.SetInterruptDelay(l, 300*time.Millisecond)
	}
	over.Store(true)
	start := time.Now()
	h.Server.PollBudget()
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Errorf("stop took %v, want it to wait for the 300ms Interrupts", elapsed)
	}
	assertNoLiveSessions(t, fake)
}

// An Interrupt that fails (here: the agent is not ready) is logged and the run
// is stopped anyway.
func TestBudget_HardLimitStopsDespiteInterruptError(t *testing.T) {
	h, fake, over, labels := startBudgetRuns(t, 2, 50*time.Millisecond)
	fake.SetInterruptErr(labels[0], agentrunner.ErrNotReady)
	fake.SetInterruptErr(labels[1], errors.New("boom"))
	over.Store(true)
	h.Server.PollBudget()
	assertNoLiveSessions(t, fake)
}

// A run that settles on its own during the stop closes its session a moment
// after the stop returns, so the check waits for it.
func assertNoLiveSessions(t *testing.T, fake *runnerfake.Runner) {
	t.Helper()
	var live []agentrunner.Session
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if live, _ = fake.List("epic-a"); len(live) == 0 {
			return
		}
	}
	t.Errorf("sessions still live: %v", live)
}
