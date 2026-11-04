package wire

// EventLimitsChanged is emitted when a stored Claude limits window really
// changed. EventBudgetChanged is emitted when the budget summary changes.
const (
	EventLimitsChanged = "limits-changed"
	EventBudgetChanged = "budget-changed"
)
