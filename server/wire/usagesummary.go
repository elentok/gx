package wire

import (
	"time"
)

// Usage modes: limits once the limits store holds any window, else spending.
// The store never expires a window, so the mode never goes back.
const (
	UsageModeLimits   = "limits"
	UsageModeSpending = "spending"
)

// UsageSpend is the spend block of the usage summary, in dollars. A *Short
// flag is set when the ledger starts after that window's start day, so the
// client writes "since <FirstDay>". FirstDay is omitted for an empty ledger.
type UsageSpend struct {
	Today            float64 `json:"today"`
	SinceSunday      float64 `json:"since_sunday"`
	SinceSundayShort bool    `json:"since_sunday_short,omitempty"`
	SinceFirst       float64 `json:"since_first"`
	SinceFirstShort  bool    `json:"since_first_short,omitempty"`
	FirstDay         string  `json:"first_day,omitempty"`
}

// UsageLimitWindow is a limit window as the menu shows it. Chip and Forecast
// are set on weekly limits only; Chip is absent until a full day of history
// exists.
type UsageLimitWindow struct {
	LimitWindow `tstype:",extends"`
	Chip        *LimitBurnChip `json:"chip,omitempty"`
	Forecast    *LimitForecast `json:"forecast,omitempty"`
}

// UsageLimits is the 5-hour and weekly window of one provider.
type UsageLimits struct {
	FiveHour *UsageLimitWindow `json:"five_hour,omitempty"`
	SevenDay *UsageLimitWindow `json:"seven_day,omitempty"`
}

// UsageSummary is the one read the web menu fetches. The spend block is always
// filled; the client picks the block to show by Mode. Codex is set only once
// the history holds a Codex sample.
type UsageSummary struct {
	Mode   string       `json:"mode"`
	Limits UsageLimits  `json:"limits"`
	Codex  *UsageLimits `json:"codex,omitempty"`
	Spend  UsageSpend   `json:"spend"`
}

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

// UsageOverrideResult is the outcome of lifting the usage gate: the windows
// lifted and when the lift ends, or a refusal.
type UsageOverrideResult struct {
	Windows []string  `json:"windows,omitempty"`
	Until   time.Time `json:"until,omitzero"`
	Refused bool      `json:"refused,omitempty"`
	Reason  string    `json:"reason,omitempty"`
	Message string    `json:"message,omitempty"`
}

// ReasonNotGated refuses a usage override when no window is gating.
const ReasonNotGated = "not-gated"

// UsageOverrideRequest names the harness whose gate to lift; empty means the
// Default harness.
type UsageOverrideRequest struct {
	Harness string `json:"harness,omitempty"`
}
