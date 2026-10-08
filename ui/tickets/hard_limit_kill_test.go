package tickets

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// writeHardLimitTicket writes a stub ticket file at the path
// ralphloop.ResolveTicketPath's glob expects, so killLiveIterations can
// resolve a Path for the stop-and-repair seam.
func writeHardLimitTicket(t *testing.T, scratchDir, epicName, identifier string) {
	t.Helper()
	path := filepath.Join(scratchDir, epicName, "issues", identifier+"-fixture.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nid: \""+identifier+"\"\nstatus: open\n---\n\nBody.\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

// withBudgetHardLimit mirrors withBudgetSoftLimit for the hard-limit fields.
func withBudgetHardLimit(t *testing.T, hardLimit float64, thresholds []float64) {
	t.Helper()
	previous := budgetConfig
	SetBudgetConfig(config.BudgetConfig{HardLimit: hardLimit, NotificationThresholds: thresholds})
	costAgg.mu.Lock()
	costAgg.hardLimitLatch.reset()
	costAgg.mu.Unlock()
	t.Cleanup(func() {
		SetBudgetConfig(previous)
		costAgg.mu.Lock()
		costAgg.hardLimitLatch.reset()
		costAgg.mu.Unlock()
	})
}

// captureStoppedIterations swaps stopIterationAndMarkNeedsRepairFn for a
// fake that records every call instead of touching real panes/tabs.
func captureStoppedIterations(t *testing.T) *[]tickets.Ticket {
	t.Helper()
	var mu sync.Mutex
	var calls []tickets.Ticket
	var grace time.Duration
	previous := stopIterationAndMarkNeedsRepairFn
	stopIterationAndMarkNeedsRepairFn = func(_ ralphloop.Deps, _ ralphloop.EventSink, _, _ string, ticket tickets.Ticket, _, _ string, g time.Duration, _ string) error {
		mu.Lock()
		calls = append(calls, ticket)
		grace = g
		mu.Unlock()
		return nil
	}
	t.Cleanup(func() {
		stopIterationAndMarkNeedsRepairFn = previous
		if grace != 0 && grace != hardLimitGrace {
			t.Fatalf("grace passed to seam = %v, want %v", grace, hardLimitGrace)
		}
	})
	return &calls
}

func TestCheckBudgetHardLimit_KillsEveryLiveIterationOnce(t *testing.T) {
	resetCostAgg(t)
	withBudgetHardLimit(t, 10.0, nil)
	sentNotifications := captureBudgetNotifications(t)
	stopped := captureStoppedIterations(t)

	landed := 0.0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return landed, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)
	scratchDir := t.TempDir()
	writeHardLimitTicket(t, scratchDir, "epic-a", "01")
	writeHardLimitTicket(t, scratchDir, "epic-a", "02")
	snap := []epicCostSnapshot{{EpicName: "epic-a", ScratchDir: scratchDir, Tickets: map[string]costTicketSnapshot{
		"01": runningTicket("", "", ralphloop.AgentClaude),
		"02": runningTicket("", "", ralphloop.AgentCodex),
	}}}

	costAgg.tick(snap) // baseline == 0, no trip
	if len(*stopped) != 0 {
		t.Fatalf("stopped before crossing the limit = %v, want none", *stopped)
	}

	landed = 12.0
	costAgg.tick(snap) // spend == 12, crosses $10

	if len(*stopped) != 2 {
		t.Fatalf("stopped = %v, want exactly 2 (including the Codex iteration)", *stopped)
	}
	if len(*sentNotifications) != 1 {
		t.Fatalf("sent = %v, want exactly 1 hard-killed notification per trip", *sentNotifications)
	}

	// A further tick still over the limit stops nothing new and sends no
	// second notification.
	landed = 14.0
	costAgg.tick(snap)
	if len(*stopped) != 2 {
		t.Fatalf("stopped after a second over-limit tick = %v, want still exactly 2", *stopped)
	}
	if len(*sentNotifications) != 1 {
		t.Fatalf("sent after a second over-limit tick = %v, want still exactly 1", *sentNotifications)
	}
}

func TestCheckBudgetHardLimit_OverrideDoesNotReinvokeSeam(t *testing.T) {
	resetCostAgg(t)
	withBudgetHardLimit(t, 10.0, nil)
	captureBudgetNotifications(t)
	stopped := captureStoppedIterations(t)

	landed := 0.0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return landed, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)
	scratchDir := t.TempDir()
	writeHardLimitTicket(t, scratchDir, "epic-a", "01")
	snap := []epicCostSnapshot{{EpicName: "epic-a", ScratchDir: scratchDir, Tickets: map[string]costTicketSnapshot{
		"01": runningTicket("", "", ralphloop.AgentClaude),
	}}}
	costAgg.tick(snap)
	landed = 12.0
	costAgg.tick(snap)
	if len(*stopped) != 1 {
		t.Fatalf("stopped = %v, want exactly 1", *stopped)
	}

	costAgg.overrideHardLimit()

	// The already-stopped iteration is still marked Running in the snapshot
	// (killLiveIterations doesn't clear it — that's the seam's job via the
	// real TabClose path); a later tick while still below the re-arm point
	// must not re-invoke the seam on it again.
	landed = 12.5
	costAgg.tick(snap)
	if len(*stopped) != 1 {
		t.Fatalf("stopped after override, below re-arm = %v, want still exactly 1", *stopped)
	}
}

