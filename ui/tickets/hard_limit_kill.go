package tickets

import (
	"fmt"
	"sync"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// hardLimitGrace is the grace period (ticket 08) passed through to ticket
// 07's stop-and-repair seam for every iteration killed by a hard-limit trip.
const hardLimitGrace = 15 * time.Second

// stopIterationAndMarkNeedsRepairFn is a test seam over ticket 07's seam.
var stopIterationAndMarkNeedsRepairFn = ralphloop.StopIterationAndMarkNeedsRepair

// budgetHardLimitKilledText is the hard-killed notification kind's text
// template (see ticket 05's comment on budgetThresholdCrossedText).
func budgetHardLimitKilledText(total, limit float64) string {
	return fmt.Sprintf("Budget alert: session spend ($%.2f) has crossed the hard limit ($%.2f) — every live iteration was stopped", total, limit)
}

// budgetHardPauseConfirmPrompt is the "p" key's confirm-dialog copy while a
// hard-limit pause is active.
func budgetHardPauseConfirmPrompt(spend, limit float64) string {
	return fmt.Sprintf("Budget hard limit reached (spend $%.2f / limit $%.2f). Override and resume the queue?", spend, limit)
}

// checkBudgetHardLimit is called from tick() right after checkBudgetSoftLimit,
// mirroring checkBudgetSoftLimit's latch-until-cleared shape (ticket 06) but
// killing every live iteration across every running epic the moment it trips
// (ticket 08), instead of merely pausing new starts.
func (a *costAggregator) checkBudgetHardLimit(total float64, snapshot []epicCostSnapshot) {
	limit := budgetConfig.HardLimit
	if limit <= 0 {
		return
	}

	a.mu.Lock()
	fired := a.hardLimitLatch.checkAndTrip(total, limit, budgetConfig.NotificationThresholds)
	a.mu.Unlock()
	if !fired {
		return
	}

	// Pause before killing to close the race window where a new
	// epic/iteration could start during the kill's grace period.
	ralphLoopRegistry.pauseHardLimit()
	killLiveIterations(snapshot)
	_, _ = sendBudgetNotificationFn(notificationsConfig, ralphloop.BudgetHardLimitKilled, budgetHardLimitKilledText(total, limit))
}

// overrideHardLimit accepts the operator's confirm-dialog override of an
// active hard-limit trip, exposed via OverrideHardLimitPause. It only
// un-pauses new starts — already-stopped iterations are never relaunched.
func (a *costAggregator) overrideHardLimit() {
	a.mu.Lock()
	a.hardLimitLatch.override(a.total)
	a.mu.Unlock()
	ralphLoopRegistry.resumeHardLimit()
}

// killLiveIterations calls ticket 07's stop-and-repair seam once per live
// iteration across every epic in snapshot, concurrently so N iterations'
// grace periods overlap instead of stacking to N*hardLimitGrace.
func killLiveIterations(snapshot []epicCostSnapshot) {
	deps := ralphloop.DefaultDeps()
	var wg sync.WaitGroup
	for _, epic := range snapshot {
		for identifier, ticket := range epic.Tickets {
			if !ticket.Running {
				continue
			}
			path := ralphloop.ResolveTicketPath(epic.ScratchDir, epic.EpicName, identifier)
			if path == "" {
				continue
			}
			t := tickets.Ticket{Identifier: identifier, Path: path}
			paneID, tabID := ticket.PaneID, ticket.TabID
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = stopIterationAndMarkNeedsRepairFn(deps, t, paneID, tabID, hardLimitGrace, "budget hard limit reached")
			}()
		}
	}
	wg.Wait()
}
