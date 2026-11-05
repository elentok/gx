package wire

// LimitsStaleAfterSeconds is how old a window's received-at may get before the
// web client shows it as stale.
const LimitsStaleAfterSeconds = 600

// LimitWindow is one rate-limit window; all times are epoch seconds.
// ReceivedAt is set by the server, never taken from a report.
type LimitWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
	ReceivedAt     int64   `json:"received_at"`
}

// LimitsReport is a report or the stored state: the windows it carries.
type LimitsReport struct {
	FiveHour *LimitWindow `json:"five_hour,omitempty"`
	SevenDay *LimitWindow `json:"seven_day,omitempty"`
}

// LimitOutcome says what a report did to one window.
type LimitOutcome string

const (
	// LimitNew filled a window that was empty.
	LimitNew LimitOutcome = "new"
	// LimitChanged moved the whole-number percentage or the reset time.
	LimitChanged LimitOutcome = "changed"
	// LimitConfirmed refreshed received-at without a visible change.
	LimitConfirmed LimitOutcome = "confirmed"
	// LimitDropped left the window alone.
	LimitDropped LimitOutcome = "dropped"
)

// LimitResult is the outcome for one window the report carried.
type LimitResult struct {
	Outcome LimitOutcome
	// PrevReceivedAt is the stored received-at before the report; 0 when none.
	PrevReceivedAt int64
}

// LimitsApplyResult has one result per window the report carried; the others
// are nil.
type LimitsApplyResult struct {
	FiveHour *LimitResult
	SevenDay *LimitResult
}

// EventLimitsChanged is emitted when a stored Claude limits window really
// changed. EventBudgetChanged is emitted when the budget summary changes.
const (
	EventLimitsChanged = "limits-changed"
	EventBudgetChanged = "budget-changed"
)
