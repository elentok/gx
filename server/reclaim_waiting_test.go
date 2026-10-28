package server_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

// markWaiting stamps the staged handle as waiting for an answer, as a server
// that was stopped while the agent waited would have saved it.
func markWaiting(t *testing.T, h *servertest.Harness, since time.Time) {
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
	runs[0]["waiting_since"] = since
	if data, err = json.Marshal(runs); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func startWaitingRestart(t *testing.T, hardLimit float64) (*servertest.Harness, string) {
	t.Helper()
	h, ticketPath, _ := startWaitingRestartFake(t, hardLimit)
	return h, ticketPath
}

// startWaitingRestartFake is startWaitingRestart that also returns the runner
// hosting the waiting agent, for tests that read its prompts.
func startWaitingRestartFake(t *testing.T, hardLimit float64, tweak ...func(*server.Config)) (*servertest.Harness, string, *headlessFake) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := &headlessFake{Runner: runnerfake.NewRunner(), verdict: nativerunner.VerdictLive}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.RunnerFor = func(_, _ string) agentrunner.Runner { return fake }
		c.BudgetHardLimit = hardLimit
		c.BudgetKillGrace = time.Millisecond
		c.BudgetPollInterval = time.Hour
		for _, f := range tweak {
			f(c)
		}
	})
	wtPath, ticketPath := leaveIteration(t, h, store, repo)
	// The agent is still alive on the runner while the server is down.
	if _, err := fake.Start(agentrunner.StartOptions{Label: "epic-a-iter-01", Cwd: wtPath}); err != nil {
		t.Fatal(err)
	}
	asHeadlessRun(t, h)
	markWaiting(t, h, time.Now().Add(-time.Hour))
	h.Restart(t)
	return h, ticketPath, fake
}

func TestReclaim_WaitingRunStaysWaitingAndUnsettled(t *testing.T) {
	t.Parallel()
	h, ticketPath := startWaitingRestart(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	// A settled run would merge (the fake agent is idle with a commit) and leave the registry.
	time.Sleep(500 * time.Millisecond)
	if runs := h.Server.Runs(); len(runs) != 1 {
		t.Errorf("Runs() = %+v, want the waiting run kept", runs)
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusClaimed {
		t.Errorf("status = %q, want claimed (not merged or paused)", tk.Status)
	}
	if data, _ := os.ReadFile(filepath.Join(h.StateDir, "runs.json")); !strings.Contains(string(data), `"waiting_since"`) {
		t.Errorf("runs.json lost waiting_since:\n%s", data)
	}
}

func TestReclaim_BudgetHardLimitStopsAWaitingRun(t *testing.T) {
	t.Parallel()
	h, ticketPath := startWaitingRestart(t, 5)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	h.Server.SetCostOf(func(server.IterationInfo) (float64, bool) { return 6, true })
	h.Server.PollBudget()
	waitFor(t, func() bool {
		data, _ := os.ReadFile(ticketPath)
		return strings.Contains(string(data), "budget-killed")
	})
	body, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(body), "status: paused") {
		t.Errorf("ticket not paused (paused):\n%s", body)
	}
}
