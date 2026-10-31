package server

// EventLimitsChanged is emitted when a stored Claude limits window really
// changed. EventBudgetChanged is emitted when the budget summary changes.
const (
	EventLimitsChanged = "limits-changed"
	EventBudgetChanged = "budget-changed"
)

// limitsVisiblyChanged reports whether a window result changes what the menu
// shows: a new or changed window, or a stale one a report just confirmed.
func limitsVisiblyChanged(r *LimitResult, now int64) bool {
	if r == nil {
		return false
	}
	switch r.Outcome {
	case LimitNew, LimitChanged:
		return true
	case LimitConfirmed:
		return r.PrevReceivedAt > 0 && now-r.PrevReceivedAt > LimitsStaleAfterSeconds
	}
	return false
}
