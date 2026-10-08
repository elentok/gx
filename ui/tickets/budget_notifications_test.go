package tickets

import (
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
)

// captureBudgetNotifications swaps sendBudgetNotificationFn for the duration
// of the test and returns the slice of texts passed to it, in call order.
func captureBudgetNotifications(t *testing.T) *[]string {
	t.Helper()
	var sent []string
	previous := sendBudgetNotificationFn
	sendBudgetNotificationFn = func(_ config.NotificationsConfig, kind ralphloop.BudgetEventKind, text string) ([]string, error) {
		sent = append(sent, text)
		return nil, nil
	}
	t.Cleanup(func() { sendBudgetNotificationFn = previous })
	return &sent
}

func withBudgetThresholds(t *testing.T, thresholds []float64) {
	t.Helper()
	previous := budgetConfig
	SetBudgetConfig(config.BudgetConfig{NotificationThresholds: thresholds})
	t.Cleanup(func() { SetBudgetConfig(previous) })
}

func TestCostAggregatorTick_JumpsPastMultipleThresholds_SendsOneMessageNamingHighest(t *testing.T) {
	resetCostAgg(t)
	withBudgetThresholds(t, []float64{5, 10, 15})
	sent := captureBudgetNotifications(t)

	landed := 0.0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) { return landed, map[string]float64{}, nil },
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)
	snap := []epicCostSnapshot{costEpic("epic-a", map[string]costTicketSnapshot{
		"01": runningTicket("", "", ralphloop.AgentClaude),
	})}

	// First tick baselines the epic at landed=0 with nothing in-flight, so
	// LiveSpend is 0 on this tick — bump landed further on tick 2 so the sum
	// actually crosses thresholds.
	costAgg.tick(snap)
	landed = 12.0
	costAgg.tick(snap)

	if len(*sent) != 1 {
		t.Fatalf("sent = %v, want exactly 1 message", *sent)
	}
	if want := budgetThresholdCrossedText(12.0, 10.0); (*sent)[0] != want {
		t.Fatalf("sent[0] = %q, want %q", (*sent)[0], want)
	}

	// Spend stays above $10 (still below $15) on the next tick: no renotify.
	costAgg.tick(snap)
	if len(*sent) != 1 {
		t.Fatalf("sent after steady-above-threshold tick = %v, want still exactly 1", *sent)
	}

	// Climbing past $15 fires exactly one more message naming $15.
	landed = 18.0
	costAgg.tick(snap)
	if len(*sent) != 2 {
		t.Fatalf("sent after climbing past $15 = %v, want exactly 2", *sent)
	}
	if want := budgetThresholdCrossedText(18.0, 15.0); (*sent)[1] != want {
		t.Fatalf("sent[1] = %q, want %q", (*sent)[1], want)
	}
}

func TestCostAggregatorTick_ReattachResetsHighWaterMark_RenotifiesOnlyAfterClimbingBackPastThreshold(t *testing.T) {
	resetCostAgg(t)
	withBudgetThresholds(t, []float64{10})
	sent := captureBudgetNotifications(t)

	landed := 12.0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) { return landed, map[string]float64{}, nil },
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)
	snap := []epicCostSnapshot{costEpic("epic-a", nil)}
	costAgg.tick(snap) // baseline == 12, LiveSpend == 0, no crossing yet
	landed = 24.0
	costAgg.tick(snap) // spend == 12, crosses $10 -> notifies once
	if len(*sent) != 1 {
		t.Fatalf("sent before reattach = %v, want exactly 1", *sent)
	}

	// Reattach: reset clears the aggregator (including the high-water-mark),
	// so the fresh baseline means the next tick's spend is 0 again even
	// though the epic is still running and still over $10 in absolute terms.
	costAgg.reset()

	costAgg.tick(snap) // fresh baseline == 24, LiveSpend == 0 again, no renotify
	if len(*sent) != 1 {
		t.Fatalf("sent right after reattach = %v, want still exactly 1", *sent)
	}

	landed = 36.0 // post-reattach spend climbs back past $10
	costAgg.tick(snap)
	if len(*sent) != 2 {
		t.Fatalf("sent after post-reattach climb past threshold = %v, want exactly 2", *sent)
	}
}
