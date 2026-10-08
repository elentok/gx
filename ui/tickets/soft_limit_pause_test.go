package tickets

import (
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
)

// withBudgetSoftLimit swaps budgetConfig for the duration of the test and
// resets costAgg's soft-limit latch/override state on both sides (the global
// costAgg persists across tests, see budget_notifications_test.go's
// withBudgetThresholds for the same budgetConfig-swap pattern).
func withBudgetSoftLimit(t *testing.T, softLimit float64, thresholds []float64) {
	t.Helper()
	previous := budgetConfig
	SetBudgetConfig(config.BudgetConfig{SoftLimit: softLimit, NotificationThresholds: thresholds})
	resetSoftLimitState(t)
	t.Cleanup(func() { SetBudgetConfig(previous) })
}

func resetSoftLimitState(t *testing.T) {
	t.Helper()
	costAgg.mu.Lock()
	costAgg.softLimitLatch.reset()
	costAgg.mu.Unlock()
	t.Cleanup(func() {
		costAgg.mu.Lock()
		costAgg.softLimitLatch.reset()
		costAgg.mu.Unlock()
	})
}

// oneRunningEpic is the tick input the soft/hard-limit tests share: one epic
// with one running Claude ticket.
func oneRunningEpic() []epicCostSnapshot {
	return []epicCostSnapshot{costEpic("epic-a", map[string]costTicketSnapshot{
		"01": runningTicket("", "", ralphloop.AgentClaude),
	})}
}

func TestCheckBudgetSoftLimit_TripsExactlyOnce(t *testing.T) {
	resetCostAgg(t)
	withBudgetSoftLimit(t, 10.0, nil)
	sent := captureBudgetNotifications(t)

	landed := 0.0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) { return landed, map[string]float64{}, nil },
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)
	snap := oneRunningEpic()

	costAgg.tick(snap) // baseline == 0, spend == 0, no trip
	if len(*sent) != 0 {
		t.Fatalf("sent before crossing the limit = %v, want none", *sent)
	}

	landed = 12.0
	costAgg.tick(snap) // spend == 12, crosses $10
	if len(*sent) != 1 {
		t.Fatalf("sent = %v, want exactly 1 message", *sent)
	}
	if want := budgetSoftLimitPausedText(12.0, 10.0); (*sent)[0] != want {
		t.Fatalf("sent[0] = %q, want %q", (*sent)[0], want)
	}

	// A further tick still over the limit sends no second notification.
	landed = 14.0
	costAgg.tick(snap)
	if len(*sent) != 1 {
		t.Fatalf("sent after a second over-limit tick = %v, want still exactly 1", *sent)
	}
}

func TestCheckBudgetSoftLimit_LatchDoesNotSelfClear(t *testing.T) {
	resetCostAgg(t)
	withBudgetSoftLimit(t, 10.0, nil)
	sent := captureBudgetNotifications(t)

	landed := 0.0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) { return landed, map[string]float64{}, nil },
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)
	snap := oneRunningEpic()
	costAgg.tick(snap) // baseline == 0
	landed = 12.0
	costAgg.tick(snap) // spend == 12, crosses $10

	landed = 0.0 // spend now reported back under the limit
	costAgg.tick(snap)
	landed = 12.0 // and back over it: a self-cleared latch would re-trip here
	costAgg.tick(snap)
	if len(*sent) != 1 {
		t.Fatalf("sent = %v, want exactly 1 (latch stays tripped when spend dips under the limit)", *sent)
	}
}

func TestCheckBudgetSoftLimit_OverrideRearms(t *testing.T) {
	cases := []struct {
		name       string
		thresholds []float64
	}{
		{name: "default config: top threshold equals soft limit", thresholds: []float64{10.0}},
		{name: "empty thresholds list", thresholds: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetCostAgg(t)
			withBudgetSoftLimit(t, 10.0, tc.thresholds)
			allSent := captureBudgetNotifications(t)
			sent := func() []string {
				var out []string
				for _, msg := range *allSent {
					if strings.Contains(msg, "soft limit") {
						out = append(out, msg)
					}
				}
				return out
			}

			landed := 0.0
			withStubbedCostReads(t,
				func(scratchDir, epicName string) (float64, map[string]float64, error) { return landed, map[string]float64{}, nil },
				func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
			)
			snap := oneRunningEpic()
			costAgg.tick(snap) // baseline == 0
			landed = 12.0
			costAgg.tick(snap) // spend == 12, trips at $10
			if len(sent()) != 1 {
				t.Fatalf("sent before override = %v, want exactly 1", sent())
			}

			costAgg.overrideSoftLimit() // override point == 12, re-arm == 12 + 1.0 == 13

			landed = 12.5 // still below the $13 re-arm point
			costAgg.tick(snap)
			if len(sent()) != 1 {
				t.Fatalf("sent while still below re-arm = %v, want still exactly 1", sent())
			}

			landed = 13.5 // climbs past the $13 re-arm point
			costAgg.tick(snap)
			if len(sent()) != 2 {
				t.Fatalf("sent after climbing past the re-arm point = %v, want exactly 2", sent())
			}
		})
	}
}
