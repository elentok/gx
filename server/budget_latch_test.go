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
