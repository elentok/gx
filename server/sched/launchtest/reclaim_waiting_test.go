package launchtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

func startWaitingRestart(t *testing.T, hardLimit float64) (*servertest.Harness, string) {
	t.Helper()
	h, ticketPath, _ := startWaitingRestartFake(t, hardLimit)
	return h, ticketPath
}

func TestReclaim_WaitingRunStaysWaitingAndUnsettled(t *testing.T) {
	t.Parallel()
	h, ticketPath := startWaitingRestart(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	// A settled run would merge (the fake agent is idle with a commit) and leave the registry.
	time.Sleep(300 * time.Millisecond) // negative wait: a settle would merge within a few polls
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
	h.Server.Budget().SetCostOf(func(wire.IterationInfo) (float64, bool) { return 6, true })
	h.Server.Budget().PollBudget()
	waitFor(t, func() bool {
		data, _ := os.ReadFile(ticketPath)
		return strings.Contains(string(data), "budget-killed")
	})
	body, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(body), "status: paused") {
		t.Errorf("ticket not paused (paused):\n%s", body)
	}
}
