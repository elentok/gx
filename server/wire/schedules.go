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

// Reasons a Schedule read is refused.
const (
	ScheduleReasonUnknown        = "unknown-schedule"
	ScheduleReasonInvalidProject = "invalid-project"
)

// ScheduleInfo is one Schedule as the list and show results carry it.
type ScheduleInfo struct {
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Project string `json:"project"`
	// Days are lowercase three-letter day names in week order.
	Days []string `json:"days"`
	// Times are HH:MM local times.
	Times []string `json:"times"`
	// Harness is empty when the Schedule follows the Default.
	Harness string `json:"harness"`
	// DefaultHarness is the name the Default resolves to for the project.
	DefaultHarness string  `json:"default_harness"`
	LateLimitHours float64 `json:"late_limit_hours"`
	// Switch is the on/off switch of this machine.
	Switch       bool `json:"switch"`
	ProjectFound bool `json:"project_found"`
	// NextRun is RFC 3339; empty when the switch is off or the project is missing.
	NextRun   string `json:"next_run"`
	UpdatedAt string `json:"updated_at"`
}

// SchedulesResult is the list of Schedules. A refusal is a result.
type SchedulesResult struct {
	Refused   bool           `json:"refused"`
	Reason    string         `json:"reason"`
	Message   string         `json:"message"`
	Schedules []ScheduleInfo `json:"schedules"`
	// Projects are the registered project names, for the Project select.
	Projects []string `json:"projects"`
}

// ScheduleShowResult is one Schedule with its prompt and Last runs.
type ScheduleShowResult struct {
	Refused  bool          `json:"refused"`
	Reason   string        `json:"reason"`
	Message  string        `json:"message"`
	Schedule ScheduleInfo  `json:"schedule"`
	Prompt   string        `json:"prompt"`
	LastRuns []ScheduleRun `json:"last_runs"`
}
