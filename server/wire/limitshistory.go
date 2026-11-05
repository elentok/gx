package wire

// LimitSample is one saved reading; times are epoch seconds. UsedPercentage is
// the raw number, not rounded.
type LimitSample struct {
	LimitID        string  `json:"limit_id"`
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
	ReceivedAt     int64   `json:"received_at"`
}

// Limit ids in the history.
const (
	LimitIDClaude5h     = "claude-5h"
	LimitIDClaudeWeekly = "claude-weekly"
	LimitIDCodex5h      = "codex-5h"
	LimitIDCodexWeekly  = "codex-weekly"
)

// LimitsReportResult is the outcome of a limits report: the windows stored
// after the merge, or a refusal. A report with no windows changes nothing and
// reads the stored windows back.
type LimitsReportResult struct {
	FiveHour *LimitWindow `json:"five_hour,omitempty"`
	SevenDay *LimitWindow `json:"seven_day,omitempty"`
	Refused  bool         `json:"refused,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	Message  string       `json:"message,omitempty"`
}

// ReasonInvalidReport refuses a limits report with a non-finite or
// out-of-range value.
const ReasonInvalidReport = "invalid-report"
