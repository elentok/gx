package wire

// ScheduleRunStatus is where one Last runs row stands.
type ScheduleRunStatus string

// The closed set of Last runs statuses. The values are literals because tygo
// cannot resolve constants from another package.
const (
	ScheduleRunQueued      ScheduleRunStatus = "queued"
	ScheduleRunRunning     ScheduleRunStatus = "running"
	ScheduleRunFinished    ScheduleRunStatus = "finished"
	ScheduleRunRanLate     ScheduleRunStatus = "ran-late"
	ScheduleRunNeedsAnswer ScheduleRunStatus = "needs-answer"
	ScheduleRunCancelled   ScheduleRunStatus = "cancelled"
	ScheduleRunSkipped     ScheduleRunStatus = "skipped"
)

// ScheduleRun is one row of a Schedule's Last runs: a One-off, or a skipped set time.
type ScheduleRun struct {
	// ScheduledFor is RFC 3339; empty for a Run now.
	ScheduledFor string `json:"scheduled_for"`
	CreatedAt    string `json:"created_at"`
	// Address is the One-off's address; empty for a skip.
	Address string            `json:"address"`
	Status  ScheduleRunStatus `json:"status"`
	// Result is the first non-empty line of the One-off's Result section.
	Result string `json:"result"`
	// Reason and ReasonText describe a skip only.
	Reason     string `json:"reason"`
	ReasonText string `json:"reason_text"`
	// AttentionAddress is the blocking ticket of a needs-attention skip.
	AttentionAddress string `json:"attention_address"`
}
