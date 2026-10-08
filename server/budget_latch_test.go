package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func startLatchServer(t *testing.T) *servertest.Harness {
	t.Helper()
	return servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.BudgetSoftLimit = 5
		c.BudgetPollInterval = time.Hour
	})
}

func TestBudgetLatch_SurvivesRestartAndOverrideLiftsIt(t *testing.T) {
	h := startLatchServer(t)
	h.Server.RecordSpend("spent", 6)
	h.Server.PollBudget()
	h.Restart(t)

	if !h.Server.BudgetStatusAt(time.Now()).BudgetPaused {
		t.Fatal("latch lost across restart")
	}
	ctx := context.Background()
	res, err := h.Client.BudgetOverride(ctx)
	if err != nil || res.Refused {
		t.Fatalf("override: %+v, %v", res, err)
	}
	if res.Budget.BudgetPaused {
		t.Error("override left the budget pause on")
	}
	h.Restart(t)
	if h.Server.BudgetStatusAt(time.Now()).BudgetPaused {
		t.Error("override did not persist")
	}
	// The override point is sticky: only spend beyond it counts toward the limit.
	h.Server.RecordSpend("more", 4)
	if h.Server.BudgetStatusAt(time.Now()).BudgetPaused {
		t.Error("spend below the fresh allowance latched again")
	}
	h.Server.RecordSpend("more2", 2)
	if !h.Server.BudgetStatusAt(time.Now()).BudgetPaused {
		t.Error("spend past the fresh allowance did not latch")
	}
}

func TestBudgetLatch_ResetsAtMidnight(t *testing.T) {
	h := startLatchServer(t)
	h.Server.RecordSpend("spent", 6)
	now := time.Now()
	if !h.Server.BudgetStatusAt(now).BudgetPaused {
		t.Fatal("not latched")
	}
	if h.Server.BudgetStatusAt(now.Add(24 * time.Hour)).BudgetPaused {
		t.Error("latch survived midnight")
	}
}

func TestBudgetOverride_RefusesWhenNothingLatched(t *testing.T) {
	h := startLatchServer(t)
	res, err := h.Client.BudgetOverride(context.Background())
	if err != nil || !res.Refused || res.Reason != server.ReasonNothingLatched {
		t.Fatalf("override = %+v, %v; want nothing-latched refusal", res, err)
	}
}

func TestBudgetLatch_IsIndependentOfUserPause(t *testing.T) {
	h := startLatchServer(t)
	ctx := context.Background()
	if _, err := h.Client.QueuePause(ctx); err != nil {
		t.Fatal(err)
	}
	h.Server.RecordSpend("spent", 6)
	if res, err := h.Client.BudgetOverride(ctx); err != nil || res.Refused {
		t.Fatalf("override: %+v, %v", res, err)
	}
	// Draining a paused queue keeps the pause, so its mode reads the user pause.
	if res, err := h.Client.QueueDrain(ctx); err != nil || res.Mode != server.ModePaused {
		t.Fatalf("mode = %+v, %v; want paused after the budget latch cleared", res, err)
	}
}

func TestBudgetStatus_ReportsOverridePoint(t *testing.T) {
	h := startLatchServer(t)
	h.Server.RecordSpend("spent", 6)
	if got := h.Server.BudgetStatusAt(time.Now()).Override; got != 0 {
		t.Fatalf("override = %v before any override, want 0", got)
	}
	res, err := h.Client.BudgetOverride(context.Background())
	if err != nil || res.Refused {
		t.Fatalf("override: %+v, %v", res, err)
	}
	if res.Budget.Override != 6 {
		t.Errorf("override = %v, want 6 (the day's total when overridden)", res.Budget.Override)
	}
}

func TestBudgetIncrease_LiftsLatchWithoutMovingOverride(t *testing.T) {
	h := startLatchServer(t)
	h.Server.RecordSpend("spent", 6)
	if !h.Server.BudgetStatusAt(time.Now()).BudgetPaused {
		t.Fatal("not latched")
	}
	soft := 10.0
	res, err := h.Client.BudgetIncrease(context.Background(), server.BudgetIncreaseRequest{Soft: &soft})
	if err != nil || res.Refused {
		t.Fatalf("increase: %+v, %v", res, err)
	}
	if res.Budget.BudgetPaused || res.Budget.SoftLimit != 10 || res.Budget.Override != 0 {
		t.Fatalf("after increase = %+v; want unpaused, soft 10, no override", *res.Budget)
	}
	// The full day's spend still counts: 6 + 5 crosses the raised limit.
	h.Server.RecordSpend("more", 5)
	if !h.Server.BudgetStatusAt(time.Now()).BudgetPaused {
		t.Error("spend past the raised limit did not latch")
	}
}

func TestBudgetIncrease_KeepsLatchWhenStillReached(t *testing.T) {
	h := startLatchServer(t)
	h.Server.RecordSpend("spent", 9)
	soft := 8.0
	res, err := h.Client.BudgetIncrease(context.Background(), server.BudgetIncreaseRequest{Soft: &soft})
	if err != nil || res.Refused {
		t.Fatalf("increase: %+v, %v", res, err)
	}
	if !res.Budget.BudgetPaused {
		t.Error("raise below today's spend lifted the latch")
	}
}

func TestBudgetIncrease_SoftPastHardRaisesHard(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.BudgetSoftLimit, c.BudgetHardLimit = 5, 7
		c.BudgetPollInterval = time.Hour
	})
	soft := 9.0
	res, err := h.Client.BudgetIncrease(context.Background(), server.BudgetIncreaseRequest{Soft: &soft})
	if err != nil || res.Refused {
		t.Fatalf("increase: %+v, %v", res, err)
	}
	if res.Budget.HardLimit != 9 {
		t.Errorf("hard = %v, want 9 (raised with soft)", res.Budget.HardLimit)
	}
}

func TestBudgetIncrease_RefusesLowerOrOffLimit(t *testing.T) {
	h := startLatchServer(t)
	ctx := context.Background()
	lower, some := 4.0, 100.0
	for name, req := range map[string]server.BudgetIncreaseRequest{
		"lower soft": {Soft: &lower},
		"off hard":   {Hard: &some},
		"empty":      {},
	} {
		res, err := h.Client.BudgetIncrease(ctx, req)
		if err != nil || !res.Refused || res.Reason != server.ReasonNotAnIncrease {
			t.Errorf("%s: %+v, %v; want not-an-increase refusal", name, res, err)
		}
	}
}
