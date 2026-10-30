package nativerunner

import "time"

// UsageFile is the usage record the statusline command writes into a pty
// agent directory.
const UsageFile = "usage.json"

// UsageLimitPercent is the used percentage at which a window counts as at its
// limit. Fixed on purpose: no config key.
const UsageLimitPercent = 98

// UsageWindow is one rate-limit window as Claude Code's statusline payload
// reports it. ResetsAt is Unix seconds.
type UsageWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

// UsageRecord is the last rate_limits payload a pty agent's statusline saw.
// A window the payload lacked is nil.
type UsageRecord struct {
	SeenAt   time.Time    `json:"seen_at"`
	FiveHour *UsageWindow `json:"five_hour,omitempty"`
	SevenDay *UsageWindow `json:"seven_day,omitempty"`
}
