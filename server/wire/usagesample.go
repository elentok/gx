package wire

import "time"

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
