package tickets

import (
	"fmt"

	"github.com/elentok/gx/ralphloop"
)

// budgetSoftLimitPausedText is the soft-paused notification kind's text
// template (see ticket 05's comment on budgetThresholdCrossedText).
func budgetSoftLimitPausedText(total, limit float64) string {
	return fmt.Sprintf("Budget alert: session spend ($%.2f) has reached the soft limit ($%.2f) — the queue is paused", total, limit)
}

// budgetPauseConfirmPrompt is the "p" key's confirm-dialog copy while a
// budget pause (soft-limit today, hard-limit once 08 lands) is active.
func budgetPauseConfirmPrompt(spend, limit float64) string {
	return fmt.Sprintf("Budget soft limit reached (spend $%.2f / limit $%.2f). Override and resume the queue?", spend, limit)
}

// checkBudgetSoftLimit is called from tick() right after
// checkBudgetThresholds, implementing ticket 06's latch-until-cleared
// soft-limit pause: once tripped, a later tick showing spend back under the
// limit never self-clears it (no "total < limit" clear path below — that
// guard only prevents a *fresh* trip). Cleared only by overrideSoftLimit
// (once spend climbs past the computed re-arm point) or by start()/stop()
// (reattach).
func (a *costAggregator) checkBudgetSoftLimit(total float64) {
	limit := budgetConfig.SoftLimit
	if limit <= 0 {
		return
	}

	a.mu.Lock()
	tripped, overridden, overridePoint := a.softLimitTripped, a.softLimitOverride, a.softLimitOverridePoint
	a.mu.Unlock()

	if overridden {
		rearm := softLimitRearmPoint(overridePoint, limit, budgetConfig.NotificationThresholds)
		if total <= rearm {
			return // still suppressed
		}
		a.mu.Lock()
		a.softLimitOverride = false
		a.softLimitTripped = false // re-arm: allow a fresh trip below
		a.mu.Unlock()
		tripped = false
	}

	if tripped || total < limit {
		return
	}

	a.mu.Lock()
	a.softLimitTripped = true
	a.mu.Unlock()
	ralphLoopRegistry.pauseSoftLimit()
	_, _ = sendBudgetNotificationFn(notificationsConfig, ralphloop.BudgetSoftLimitPaused, budgetSoftLimitPausedText(total, limit))
}

// softLimitRearmPoint implements ticket 06's re-arm formula: min(next
// configured notification threshold strictly above overridePoint,
// overridePoint + 10% of softLimit) — well-defined even with no threshold
// above overridePoint (falls back to the +10% arm) or an empty thresholds
// list.
func softLimitRearmPoint(overridePoint, softLimit float64, thresholds []float64) float64 {
	rearm := overridePoint + 0.1*softLimit
	for _, th := range thresholds {
		if th > overridePoint && th < rearm {
			rearm = th
		}
	}
	return rearm
}

// overrideSoftLimit accepts the operator's confirm-dialog override of an
// active soft-limit trip, exposed via OverrideSoftLimitPause.
func (a *costAggregator) overrideSoftLimit() {
	a.mu.Lock()
	a.softLimitOverride = true
	a.softLimitOverridePoint = a.total
	a.mu.Unlock()
	ralphLoopRegistry.resumeSoftLimit()
}
