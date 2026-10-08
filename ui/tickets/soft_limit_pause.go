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

// checkBudgetSoftLimit is called from tick() right after
// checkBudgetThresholds, implementing ticket 06's latch-until-cleared
// soft-limit pause: once tripped, a later tick showing spend back under the
// limit never self-clears it (no "total < limit" clear path below — that
// guard only prevents a *fresh* trip). Cleared only by overrideSoftLimit
// (once spend climbs past the computed re-arm point) or by reset()
// (reattach).
func (a *costAggregator) checkBudgetSoftLimit(total float64) {
	limit := budgetConfig.SoftLimit
	if limit <= 0 {
		return
	}

	a.mu.Lock()
	fired := a.softLimitLatch.checkAndTrip(total, limit, budgetConfig.NotificationThresholds)
	a.mu.Unlock()
	if !fired {
		return
	}

	_, _ = sendBudgetNotificationFn(notificationsConfig, ralphloop.BudgetSoftLimitPaused, budgetSoftLimitPausedText(total, limit))
}

// overrideSoftLimit accepts the operator's confirm-dialog override of an
// active soft-limit trip.
func (a *costAggregator) overrideSoftLimit() {
	a.mu.Lock()
	a.softLimitLatch.override(a.total)
	a.mu.Unlock()
}
