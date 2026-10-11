package server_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func budgetLimits(soft, hard float64) map[string]any {
	return map[string]any{"budget": map[string]any{"soft-limit": soft, "hard-limit": hard}}
}

func startBudgetReloadServer(t *testing.T, soft, hard float64) (*servertest.Harness, string, string) {
	t.Helper()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) {
		c.BudgetSoftLimit, c.BudgetHardLimit = soft, hard
		c.BudgetPollInterval = time.Hour
	})
	return h, path, store
}

// Seam: a raised limit shows in the budget status and lifts a latch spend no
// longer reaches.
func TestConfigReload_RaisedBudgetLimitsLiftLatches(t *testing.T) {
	h, path, store := startBudgetReloadServer(t, 5, 7)
	h.Server.RecordSpend("spent", 8)
	if st := h.Server.BudgetStatusAt(time.Now()); !st.HardLatched {
		t.Fatalf("status = %+v, want hard latched", st)
	}

	servertest.WriteConfig(t, path, store, budgetLimits(5, 20))
	res := reload(t, h)
	if res.Refused || !slices.Contains(res.Changed, "budget.hard-limit") || slices.Contains(res.Changed, "budget.soft-limit") {
		t.Fatalf("reload = %+v, want budget.hard-limit changed and soft-limit not", res)
	}
	st := h.Server.BudgetStatusAt(time.Now())
	if st.HardLimit != 20 || st.HardLatched || !st.BudgetPaused {
		t.Fatalf("after hard raise = %+v, want hard 20 unlatched, soft still latched", st)
	}

	servertest.WriteConfig(t, path, store, budgetLimits(10, 20))
	reload(t, h)
	if st := h.Server.BudgetStatusAt(time.Now()); st.SoftLimit != 10 || st.BudgetPaused {
		t.Errorf("after soft raise = %+v, want soft 10 unpaused", st)
	}
}

// Seam: a reload editing only hard-limit keeps a soft limit raised by
// `gx budget increase`.
func TestConfigReload_KeepsIncreaseOnUneditedField(t *testing.T) {
	h, path, store := startBudgetReloadServer(t, 5, 20)
	soft := 10.0
	if res, err := h.Client.BudgetIncrease(context.Background(), server.BudgetIncreaseRequest{Soft: &soft}); err != nil || res.Refused {
		t.Fatalf("increase: %+v, %v", res, err)
	}

	servertest.WriteConfig(t, path, store, budgetLimits(5, 30))
	reload(t, h)
	if st := h.Server.BudgetStatusAt(time.Now()); st.SoftLimit != 10 || st.HardLimit != 30 {
		t.Errorf("status = %+v, want soft 10 (kept increase), hard 30", st)
	}
}

// Seam: a lowered soft limit already exceeded latches at the next check.
func TestConfigReload_LoweredSoftLimitLatchesAtNextCheck(t *testing.T) {
	h, path, store := startBudgetReloadServer(t, 10, 0)
	h.Server.RecordSpend("spent", 6)
	if h.Server.BudgetStatusAt(time.Now()).BudgetPaused {
		t.Fatal("latched before the reload")
	}

	servertest.WriteConfig(t, path, store, budgetLimits(5, 0))
	reload(t, h)
	if !h.Server.BudgetStatusAt(time.Now()).BudgetPaused {
		t.Error("lowered soft limit below spend did not latch")
	}
}

// Seam: a hard limit lowered to or below spend warns and never latches hard,
// so it stops no agent.
func TestConfigReload_LoweredHardLimitBelowSpendWarnsWithoutLatching(t *testing.T) {
	h, path, store := startBudgetReloadServer(t, 5, 20)
	h.Server.RecordSpend("spent", 8)

	servertest.WriteConfig(t, path, store, budgetLimits(5, 6))
	res := reload(t, h)
	if res.Refused || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "budget.hard-limit") {
		t.Fatalf("reload = %+v, want one budget.hard-limit warning", res)
	}
	h.Server.PollBudget()
	if st := h.Server.BudgetStatusAt(time.Now()); st.HardLimit != 6 || st.HardLatched {
		t.Errorf("status = %+v, want hard 6 unlatched", st)
	}

	// Raising it back above spend ends the hold, so later spend latches as usual.
	servertest.WriteConfig(t, path, store, budgetLimits(5, 9))
	if res := reload(t, h); len(res.Warnings) != 0 {
		t.Fatalf("raise = %+v, want no warning", res)
	}
	h.Server.RecordSpend("more", 10)
	if !h.Server.BudgetStatusAt(time.Now()).HardLatched {
		t.Error("spend past the raised hard limit did not latch")
	}
}
