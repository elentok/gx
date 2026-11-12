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
	ScheduledFor     string            `json:"scheduled_for"` // RFC 3339; empty for a Run now
	CreatedAt        string            `json:"created_at"`
	Address          string            `json:"address"` // the One-off's; empty for a skip
	Status           ScheduleRunStatus `json:"status"`
	Result           string            `json:"result"`            // first non-empty line of the One-off's Result section
	Reason           string            `json:"reason"`            // skip only
	ReasonText       string            `json:"reason_text"`       // skip only
	AttentionAddress string            `json:"attention_address"` // blocking ticket of a needs-attention skip
}

// Reasons a Schedule read or edit is refused. ProjectReadOnly refuses an edit
// that changes the project.
const (
	ScheduleReasonUnknown         = "unknown-schedule"
	ScheduleReasonInvalidProject  = "invalid-project"
	ScheduleReasonProjectReadOnly = "project-read-only"
)

// ScheduleInfo is one Schedule as the list and show results carry it.
type ScheduleInfo struct {
	Name           string   `json:"name"`
	Slug           string   `json:"slug"`
	Project        string   `json:"project"`
	Days           []string `json:"days"`            // lowercase three-letter names in week order
	Times          []string `json:"times"`           // HH:MM local
	Harness        string   `json:"harness"`         // empty when the Schedule follows the Default
	DefaultHarness string   `json:"default_harness"` // what the Default resolves to for the project
	LateLimitHours float64  `json:"late_limit_hours"`
	Switch         bool     `json:"switch"` // the on/off switch of this machine
	ProjectFound   bool     `json:"project_found"`
	NextRun        string   `json:"next_run"` // RFC 3339; empty when the switch is off or the project is missing
	UpdatedAt      string   `json:"updated_at"`
}

// SchedulesResult is the list of Schedules. A refusal is a result.
type SchedulesResult struct {
	Refused   bool           `json:"refused"`
	Reason    string         `json:"reason"`
	Message   string         `json:"message"`
	Schedules []ScheduleInfo `json:"schedules"`
	Projects  []string       `json:"projects"` // registered project names, for the Project select
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

// EventSchedulesChanged is emitted with "<project>:<slug>" after a Schedule is written.
const EventSchedulesChanged = "schedules-changed"

// ScheduleCreateRequest is the body of POST /v1/schedules/create. Days are
// lowercase three-letter names. An empty Project means scratch, an empty
// Harness the Default, a nil LateLimitHours two hours.
type ScheduleCreateRequest struct {
	Name           string   `json:"name"`
	Prompt         string   `json:"prompt"`
	Days           []string `json:"days"`
	Times          []string `json:"times"`
	Project        string   `json:"project"`
	Harness        string   `json:"harness"`
	LateLimitHours *float64 `json:"late_limit_hours"`
}

// ScheduleSwitchRequest is the body of POST /v1/schedules/switch (result: show payload).
type ScheduleSwitchRequest struct {
	Project string `json:"project"`
	Slug    string `json:"slug"`
	On      bool   `json:"on"`
}

// ScheduleDeleteRequest is the body of POST /v1/schedules/delete.
type ScheduleDeleteRequest struct {
	Project string `json:"project"`
	Slug    string `json:"slug"`
}

// ScheduleDeleteResult is the result of POST /v1/schedules/delete.
type ScheduleDeleteResult struct {
	Refused bool   `json:"refused"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// ScheduleEditRequest is the body of POST /v1/schedules/edit. Project and
// Slug name the Schedule; a nil field is left as it is. NewProject exists
// only so a project change can be refused.
type ScheduleEditRequest struct {
	Project        string   `json:"project"`
	Slug           string   `json:"slug"`
	NewProject     *string  `json:"new_project"`
	Name           *string  `json:"name"`
	Prompt         *string  `json:"prompt"`
	Days           []string `json:"days"`
	Times          []string `json:"times"`
	Harness        *string  `json:"harness"`
	LateLimitHours *float64 `json:"late_limit_hours"`
}

// ScheduleRunNowRequest is the body of POST /v1/schedules/run-now.
type ScheduleRunNowRequest struct {
	Project string `json:"project"`
	Slug    string `json:"slug"`
}

// ScheduleRunNowResult is the result of POST /v1/schedules/run-now. Address is
// the new One-off; a refusal carries Reason and Message, and AttentionAddress
// names the previous run's ticket when it needs attention.
type ScheduleRunNowResult struct {
	Address          string `json:"address"`
	Status           string `json:"status"`
	Refused          bool   `json:"refused"`
	Reason           string `json:"reason"`
	Message          string `json:"message"`
	AttentionAddress string `json:"attention_address"`
}
