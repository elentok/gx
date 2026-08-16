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
	costAgg.hardLimitTripped = false
	costAgg.hardLimitOverride = false
	costAgg.hardLimitOverridePoint = 0
	costAgg.mu.Unlock()
	t.Cleanup(func() {
		SetBudgetConfig(previous)
		costAgg.mu.Lock()
		costAgg.hardLimitTripped = false
		costAgg.hardLimitOverride = false
		costAgg.hardLimitOverridePoint = 0
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
	stopIterationAndMarkNeedsRepairFn = func(_ ralphloop.Deps, ticket tickets.Ticket, _, _ string, g time.Duration, _ string) error {
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
	r := startTestRegistry(t)
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
	if _, ok := r.tryStart("epic-a", 0, 5, scratchDir); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	writeHardLimitTicket(t, scratchDir, "epic-a", "01")
	writeHardLimitTicket(t, scratchDir, "epic-a", "02")
	startRunningTicket(t, r, "epic-a", "01", "", "", ralphloop.AgentClaude)
	startRunningTicket(t, r, "epic-a", "02", "", "", ralphloop.AgentCodex)

	costAgg.tick() // baseline == 0, no trip
	if len(*stopped) != 0 {
		t.Fatalf("stopped before crossing the limit = %v, want none", *stopped)
	}

	landed = 12.0
	costAgg.tick() // spend == 12, crosses $10

	if !r.isHardLimitPaused() {
		t.Fatal("expected hard-limit pause after crossing the limit")
	}
	if len(*stopped) != 2 {
		t.Fatalf("stopped = %v, want exactly 2 (including the Codex iteration)", *stopped)
	}
	if len(*sentNotifications) != 1 {
		t.Fatalf("sent = %v, want exactly 1 hard-killed notification per trip", *sentNotifications)
	}

	// A further tick still over the limit stops nothing new and sends no
	// second notification.
	landed = 14.0
	costAgg.tick()
	if len(*stopped) != 2 {
		t.Fatalf("stopped after a second over-limit tick = %v, want still exactly 2", *stopped)
	}
	if len(*sentNotifications) != 1 {
		t.Fatalf("sent after a second over-limit tick = %v, want still exactly 1", *sentNotifications)
	}
}

func TestCheckBudgetHardLimit_RefusesNewStartsDuringAndAfterKill(t *testing.T) {
	r := startTestRegistry(t)
	withBudgetHardLimit(t, 10.0, nil)
	captureBudgetNotifications(t)
	captureStoppedIterations(t)

	landed := 0.0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return landed, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)
	if _, ok := r.tryStart("epic-a", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	startRunningTicket(t, r, "epic-a", "01", "", "", ralphloop.AgentClaude)
	costAgg.tick()
	landed = 12.0
	costAgg.tick()

	if _, ok := r.tryStart("epic-b", 0, 5, t.TempDir()); ok {
		t.Fatal("tryStart(epic-b): want refused while hard-limit paused")
	}

	landed = 14.0
	costAgg.tick() // still over the limit, well after the kill completed
	if _, ok := r.tryStart("epic-b", 0, 5, t.TempDir()); ok {
		t.Fatal("tryStart(epic-b): want still refused after the kill")
	}
}

func TestCheckBudgetHardLimit_OverridePermitsNewStartsWithoutReinvokingSeam(t *testing.T) {
	r := startTestRegistry(t)
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
	if _, ok := r.tryStart("epic-a", 0, 5, scratchDir); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	writeHardLimitTicket(t, scratchDir, "epic-a", "01")
	startRunningTicket(t, r, "epic-a", "01", "", "", ralphloop.AgentClaude)
	costAgg.tick()
	landed = 12.0
	costAgg.tick()
	if len(*stopped) != 1 {
		t.Fatalf("stopped = %v, want exactly 1", *stopped)
	}

	costAgg.overrideHardLimit()
	if r.isHardLimitPaused() {
		t.Fatal("expected override to clear the hard-limit pause")
	}
	if _, ok := r.tryStart("epic-b", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-b) after override: want success")
	}
	t.Cleanup(func() { r.finish("epic-b", nil) })

	// The already-stopped epic-a iteration is still marked Running in the
	// registry snapshot (killLiveIterations doesn't clear it — that's the
	// seam's job via the real TabClose path); a later tick while still
	// below the re-arm point must not re-invoke the seam on it again.
	landed = 12.5
	costAgg.tick()
	if len(*stopped) != 1 {
		t.Fatalf("stopped after override, below re-arm = %v, want still exactly 1", *stopped)
	}
}

func TestCheckBudgetHardLimit_IndependentFromSoftLimitAndManualPause(t *testing.T) {
	r := startTestRegistry(t)
	previous := budgetConfig
	SetBudgetConfig(config.BudgetConfig{SoftLimit: 5.0, HardLimit: 10.0})
	t.Cleanup(func() { SetBudgetConfig(previous) })
	resetSoftLimitState(t)
	costAgg.mu.Lock()
	costAgg.hardLimitTripped = false
	costAgg.hardLimitOverride = false
	costAgg.hardLimitOverridePoint = 0
	costAgg.mu.Unlock()
	captureBudgetNotifications(t)
	captureStoppedIterations(t)

	landed := 0.0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return landed, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)
	if _, ok := r.tryStart("epic-a", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	startRunningTicket(t, r, "epic-a", "01", "", "", ralphloop.AgentClaude)
	costAgg.tick()

	landed = 12.0
	costAgg.tick() // crosses both soft ($5) and hard ($10)
	if !r.isSoftLimitPaused() || !r.isHardLimitPaused() {
		t.Fatal("expected both soft and hard limit pauses to be set")
	}

	r.pause()
	costAgg.overrideHardLimit() // must not clear the soft-limit or manual pause
	if !r.isSoftLimitPaused() {
		t.Fatal("expected hard-limit override to leave the soft-limit pause untouched")
	}
	if !r.isPaused() {
		t.Fatal("expected hard-limit override to leave the manual pause untouched")
	}
	if r.isHardLimitPaused() {
		t.Fatal("expected hard-limit override to clear the hard-limit pause")
	}
}
