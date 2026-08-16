package tickets

import (
	"fmt"
	"sort"

	"github.com/elentok/gx/ralphloop"
)

// sendBudgetNotificationFn is a test seam over ralphloop.SendBudgetNotification.
var sendBudgetNotificationFn = ralphloop.SendBudgetNotification

// budgetThresholdCrossedText is the threshold-crossed kind's text template.
// Soft-paused/hard-killed get their own templates in later tickets (06/08).
func budgetThresholdCrossedText(total, threshold float64) string {
	return fmt.Sprintf("Budget alert: session spend has crossed $%.2f (total: $%.2f)", threshold, total)
}

// checkBudgetThresholds is called on every poller tick, right after tick()
// recomputes the live total — no separate timer (see ticket 05's "What to
// build"). A tick that jumps past several configured thresholds at once
// collapses to a single notification naming the current total and the
// highest threshold just crossed. budgetHighWaterMark is the in-memory dedup
// state: once a threshold has fired, it never re-fires while spend stays
// above it, and only resets on reattach (costAggregator.start/stop).
func (a *costAggregator) checkBudgetThresholds(total float64) {
	thresholds := append([]float64(nil), budgetConfig.NotificationThresholds...)
	if len(thresholds) == 0 {
		return
	}
	sort.Float64s(thresholds)

	a.mu.Lock()
	highWaterMark := a.budgetHighWaterMark
	crossed := 0.0
	for _, threshold := range thresholds {
		if total >= threshold && threshold > highWaterMark {
			crossed = threshold
		}
	}
	if crossed == 0 {
		a.mu.Unlock()
		return
	}
	a.budgetHighWaterMark = crossed
	a.mu.Unlock()

	// Best-effort: a failed send/log shouldn't crash the poller — the caller
	// (tick) has nowhere useful to surface an error to.
	_, _ = sendBudgetNotificationFn(notificationsConfig, ralphloop.BudgetThresholdCrossed, budgetThresholdCrossedText(total, crossed))
}
