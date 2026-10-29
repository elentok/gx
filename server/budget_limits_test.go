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
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.BudgetSoftLimit = 5
	})
	start, _, _ := registerTurn(h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.TicketPause(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("pause = %+v, %v", res, err)
	}
	h.Server.RecordSpend("spent", 6)

	if res, err := h.Client.TicketRetry(ctx, "proj:epic-a/01", false); err != nil || !res.Refused || res.Reason != server.ReasonBudgetLimit || !strings.Contains(res.Message, "budget") {
		t.Errorf("retry = %+v, %v; want refusal %s naming the budget", res, err, server.ReasonBudgetLimit)
	}
	if len(*start) != 0 {
		t.Errorf("agent started past the soft limit: %v", *start)
	}
	if res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "finish up"); err != nil || !res.Refused || res.Reason != server.ReasonBudgetLimit {
		t.Errorf("nudge = %+v, %v; want refusal %s", res, err, server.ReasonBudgetLimit)
	}
}

// A nudge into a paused ticket's still-open pane (recovery's R7 does this)
// sets an agent working outside any run; it must be metered and the hard
// limit must stop it like any other live agent.
func TestBudget_NudgedPausedPaneIsMeteredAndKilled(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	h := servertest.StartWithStore(t, store, servertest.WithHerdr(servertest.UntilPtyAsDefault), func(c *server.Config) {
		c.BudgetHardLimit = 5
		c.BudgetKillGrace = 50 * time.Millisecond
		c.BudgetPollInterval = time.Hour
	})
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "compaction timed out"); err != nil {
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
	servertest.WaitFor(t, 10*time.Second, "the hard limit to stop the nudged pane", func() bool {
		h.Server.PollBudget()
		return len(h.Server.Runs()) == 0
	})
	if !closed.Load() {
		t.Error("the hard limit did not close the nudged pane")
	}
	tk, err := schema.ParseTicket(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertPaused(t, tk, schema.PauseClassRepair, schema.PauseBudgetKilled)
}

func TestBudget_SoftLimitStopsNewStarts(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.BudgetSoftLimit = 5
	})
	registerTurn(h)
	h.Server.RecordSpend("spent", 6)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	time.Sleep(250 * time.Millisecond) // negative wait: a launch would have come within a few 50ms polls
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Fatalf("runs = %+v, want none past the soft limit", runs)
	}
}

func TestBudget_SpendInTwoProjectsSumsTowardOneLimit(t *testing.T) {
	t.Parallel()
	store, repoA, repoB := t.TempDir(), testutil.TempRepo(t), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj-a", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj-b", "epic-b", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj-a", repoA)
	servertest.SetProjectRepo(t, store, "proj-b", repoB)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.BudgetSoftLimit = 5
	})
	registerTurn(h)
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
	time.Sleep(250 * time.Millisecond) // negative wait: a launch would have come within a few 50ms polls
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Fatalf("runs = %+v, want none in either project past the soft limit", runs)
	}
}

func TestBudget_HardLimitStopsLivePaneAndPausesBudgetKilled(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithHerdr(servertest.HerdrIsTheSubject), func(c *server.Config) {
		c.BudgetHardLimit = 5
		c.BudgetKillGrace = time.Millisecond
		c.BudgetPollInterval = time.Hour
	})
	registerTurn(h)
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
	servertest.WaitFor(t, 10*time.Second, "a live run", func() bool { return len(h.Server.Runs()) > 0 })
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	servertest.WaitFor(t, 10*time.Second, "the ticket to be budget-killed", func() bool {
		h.Server.PollBudget()
		data, _ := os.ReadFile(path)
		return strings.Contains(string(data), "budget-killed")
	})
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "status: paused") || !strings.Contains(string(data), "budget-killed") {
		t.Fatalf("ticket not paused budget-killed:\n%s", data)
	}
	if len(sent) == 0 || !strings.Contains(sent[0], "ctrl+c") {
		t.Errorf("send-keys = %v, want ctrl+c", sent)
	}
	if len(closed) == 0 {
		t.Error("pane was not closed")
	}
}

// An agent that keeps spending after the ctrl+c is closed and paused
// budget-killed, and its run's own finish must not overwrite that pause.
func TestBudget_HardLimitStopsRunThroughRunner(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	cost := 0.0
	testBudgetKillPausesBudgetKilled(t, func() float64 { mu.Lock(); defer mu.Unlock(); cost += 3; return cost }, servertest.TicketOpts{})
}

// An agent that goes quiet on the ctrl+c is paused budget-killed too, not
// left to its own finish (which would pause it zero-commit, a kind recovery
// relaunches past the hard limit).
func TestBudget_HardLimitPausesAQuietRunBudgetKilledToo(t *testing.T) {
	t.Parallel()
	testBudgetKillPausesBudgetKilled(t, func() float64 { return 6 }, servertest.TicketOpts{})
}

func testBudgetKillPausesBudgetKilled(t *testing.T, cost func() float64, ticket servertest.TicketOpts) {
	t.Helper()
	runBudgetKill(t, cost, ticket, nil)
}

// A hard-limit kill closes the run's open question with budget-killed, before
// the pause status.
func TestBudget_HardLimitKillClosesTheOpenQuestion(t *testing.T) {
	t.Parallel()
	path := runBudgetKill(t, func() float64 { return 6 }, servertest.TicketOpts{}, func(path string) {
		if _, err := tickets.AppendLog(path, tickets.LogEvent{Type: tickets.EventPauseQuestion, By: "gx", Summary: "q?"}); err != nil {
			t.Fatal(err)
		}
	})
	evs, err := tickets.ReadLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 || evs[1].Type != tickets.EventPauseClosed || evs[1].Question != 1 || evs[1].Reason != "budget-killed" ||
		evs[2].Type != tickets.EventPauseRepair {
		t.Errorf("log = %+v, want the question, pause.closed{question 1, budget-killed}, then the repair round", evs)
		return
	}
	if evs[1].Source != tickets.SourceGx || evs[1].How != tickets.HowAuto {
		t.Errorf("pause.closed = %+v, want source gx, how auto", evs[1])
	}
}

func TestBudget_HardLimitKillWithoutQuestionOpensOnlyARepairRound(t *testing.T) {
	t.Parallel()
	path := runBudgetKill(t, func() float64 { return 6 }, servertest.TicketOpts{}, nil)
	if evs, _ := tickets.ReadLog(path); len(evs) != 1 || evs[0].Type != tickets.EventPauseRepair {
		t.Errorf("log = %+v, want one pause.repair", evs)
	}
}

// runBudgetKill runs one ticket past the hard limit, seeding its log first
// when seed is set, and returns the ticket's path once it is paused.
func runBudgetKill(t *testing.T, cost func() float64, ticket servertest.TicketOpts, seed func(path string)) string {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", ticket)
	if seed != nil {
		seed(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
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
	servertest.WaitFor(t, 10*time.Second, "a live run", func() bool { return len(h.Server.Runs()) > 0 })
	// Poll until the run has settled, and judge the ticket only then: the
	// run's own finish is what used to overwrite the budget pause.
	servertest.WaitFor(t, 10*time.Second, "the run to settle", func() bool {
		h.Server.PollBudget()
		return len(h.Server.Runs()) == 0
	})
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	tk, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	assertPaused(t, tk, schema.PauseClassRepair, schema.PauseBudgetKilled)
	if t.Failed() {
		data, _ := os.ReadFile(path)
		t.Fatalf("ticket:\n%s", data)
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
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.BudgetHardLimit = 5
		c.BudgetKillGrace = grace
		c.BudgetPollInterval = time.Hour
		c.PollInterval = 50 * time.Millisecond
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
	servertest.WaitFor(t, 10*time.Second, fmt.Sprintf("%d live sessions", n), func() bool {
		live, _ := fake.List("epic-a")
		return len(live) >= n
	})
	return h, fake, &over, labels
}

// The grace counts from the stop decision and the Interrupts run side by side:
// stopping N slow runs takes about the grace, not the sum of their Interrupts.
func TestBudget_HardLimitInterruptsRunInParallelInsideGrace(t *testing.T) {
	t.Parallel()
	h, fake, over, labels := startBudgetRuns(t, 3, 600*time.Millisecond)
	for _, l := range labels {
		fake.SetInterruptDelay(l, 500*time.Millisecond)
	}
	over.Store(true)
	start := time.Now()
	h.Server.PollBudget()
	elapsed := time.Since(start)
	// The upper bound sits well below the serial total so a loaded machine
	// does not look like serial Interrupts.
	if elapsed < 600*time.Millisecond || elapsed >= 1200*time.Millisecond {
		t.Errorf("stop took %v, want about the 600ms grace (serial Interrupts would take 1500ms)", elapsed)
	}
	assertNoLiveSessions(t, fake)
}

// A grace shorter than the Interrupts does not cut them off: Stop waits for
// every Interrupt to return.
func TestBudget_HardLimitStopWaitsForSlowInterrupts(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	servertest.WaitFor(t, 5*time.Second, "no live sessions", func() bool {
		live, _ := fake.List("epic-a")
		return len(live) == 0
	})
}
