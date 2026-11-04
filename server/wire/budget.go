package wire

// BudgetStatus is today's spend against the configured limits, in dollars. A
// zero limit means that limit is off. Shared by /v1/budget and the snapshot.
// BudgetPaused is the budget's hold on new starts; it is independent of the
// operator's queue pause.
type BudgetStatus struct {
	Day          string  `json:"day"`
	Total        float64 `json:"total"`
	SoftLimit    float64 `json:"soft_limit"`
	HardLimit    float64 `json:"hard_limit"`
	BudgetPaused bool    `json:"budget_paused"`
	HardLatched  bool    `json:"hard_latched"`
	// Override is today's total at the last override; zero means none today.
	// Limits count only spend beyond it.
	Override float64 `json:"override,omitempty"`
	// Projects is each project's share of Total.
	Projects map[string]float64 `json:"projects,omitempty"`
	// Roots is each root's ("project:epic") share of Total.
	Roots map[string]float64 `json:"roots,omitempty"`
}

// BudgetResult is the outcome of a budget write: the status after it, or a
// refusal.
type BudgetResult struct {
	Budget  *BudgetStatus `json:"budget,omitempty"`
	Refused bool          `json:"refused,omitempty"`
	Reason  string        `json:"reason,omitempty"`
	Message string        `json:"message,omitempty"`
}

// ReasonNothingLatched refuses an override when no limit is holding anything.
const ReasonNothingLatched = "nothing-latched"

// BudgetIncreaseRequest raises the live daily limits, in dollars. A nil field
// leaves that limit as it is.
type BudgetIncreaseRequest struct {
	Soft *float64 `json:"soft,omitempty"`
	Hard *float64 `json:"hard,omitempty"`
}

// ReasonNotAnIncrease refuses a limit that is off or not raised.
const ReasonNotAnIncrease = "not-an-increase"
