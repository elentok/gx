package wire

// EventTriageChanged is emitted with the project name as its address when a
// project's live Triage record changed.
const EventTriageChanged = "triage-changed"

// TriageChangedRequest is the body of POST /v1/triage/changed: a CLI verb's
// "this project's Triage record changed" ping.
type TriageChangedRequest struct {
	Project string `json:"project"`
}

// Refusal reasons of the triage open route, beside the session ones.
const (
	// ReasonTriageApprove: the Triage waits at the approve step, where the page
	// is the way on and no session runs.
	ReasonTriageApprove = "triage-at-approve"
	// ReasonTriageNoFollowUps: the project has no open follow-up to triage.
	ReasonTriageNoFollowUps = "triage-no-follow-ups"
	// ReasonTriagePhase: the record's step or the row's state does not allow a
	// row or abandon write, or the merge target is not a ready or map row.
	ReasonTriagePhase = "triage-phase"
	// ReasonTriageAnswer: an answer names no question of the row, or is not one
	// of its question's options.
	ReasonTriageAnswer = "triage-answer"
	// ReasonTriageAlreadyApproved: the Triage was approved by another call.
	ReasonTriageAlreadyApproved = "triage-already-approved"
	// ReasonTriageNoLive: the project has no live Triage.
	ReasonTriageNoLive = "triage-no-live"
)

// TriageOpenRequest is the body of POST /v1/triage/open. New stops the
// Triage's live session and starts a fresh one; State is the token of the live
// session the web saw, required on the web mux for New.
type TriageOpenRequest struct {
	Project string `json:"project"`
	New     bool   `json:"new,omitempty"`
	State   string `json:"state,omitempty"`
}

// TriageChangedResult acknowledges a ping.
type TriageChangedResult struct {
	Project string `json:"project"`
}

// TriageState is where a Triage record is in its life; none means the project
// has had no Triage.
type TriageState string

const (
	TriageLive      TriageState = "live"
	TriageDone      TriageState = "done"
	TriageAbandoned TriageState = "abandoned"
	TriageNone      TriageState = "none"
)

// TriageQuestion is one question about a row. Title is the first line of the
// question and Body the rest.
type TriageQuestion struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Body        string   `json:"body,omitempty"`
	Kind        string   `json:"kind"`
	Options     []string `json:"options,omitempty"`
	Recommended string   `json:"recommended,omitempty"`
	// Auto marks a low-risk question the agent decided: it holds its default as the answer until the person changes it.
	Auto bool `json:"auto,omitempty"`
}

// TriageRow is one follow-up with everything the Triage learned about it.
type TriageRow struct {
	Address   string           `json:"address"`
	Title     string           `json:"title"`
	Category  string           `json:"category,omitempty"`
	Verdict   string           `json:"verdict,omitempty"`
	Evidence  string           `json:"evidence,omitempty"`
	Files     []string         `json:"files,omitempty"`
	Questions []TriageQuestion `json:"questions,omitempty"`
	// Travels holds the design questions: never asked, they go into the map-epic ticket.
	Travels []TriageQuestion `json:"travels,omitempty"`
	// Answers maps a question id to the person's answer.
	Answers map[string]string `json:"answers,omitempty"`
	// Action and Destination are the plan's, replaced by the person's when set.
	Action        string `json:"action,omitempty"`
	Destination   string `json:"destination,omitempty"`
	PersonChanged bool   `json:"person_changed,omitempty"`
	Gone          bool   `json:"gone,omitempty"`
	WriteState    string `json:"write_state,omitempty"`
	// NewAddresses are every ticket the row became.
	NewAddresses []string `json:"new_addresses,omitempty"`
	Refusal      string   `json:"refusal,omitempty"`
	CanKeep      bool     `json:"can_keep,omitempty"`
	CanRetry     bool     `json:"can_retry,omitempty"`
	// MergeOrphan marks a row set to merge into a row that is no longer a
	// ready or map epic; approve stays refused until it is re-pointed.
	MergeOrphan bool `json:"merge_orphan,omitempty"`
	// Group is set on every member of a group Plan proposed to promote as one
	// ticket; members share its value. OverlapWith lists the rows this row
	// waits for when it is not grouped.
	Group       string   `json:"group,omitempty"`
	OverlapWith []string `json:"overlap_with,omitempty"`
}

// TriageRowRequest is the body of POST /v1/triage/row. It replaces the row's
// person fields; Revision is the record revision the web read. Destination is
// an epic, a typed new map epic name, or the address of the row merged into.
type TriageRowRequest struct {
	Project      string            `json:"project"`
	Revision     int               `json:"revision"`
	Address      string            `json:"address"`
	Action       string            `json:"action,omitempty"`
	Destination  string            `json:"destination,omitempty"`
	Answers      map[string]string `json:"answers,omitempty"`
	KeepForLater bool              `json:"keep_for_later,omitempty"`
}

// TriageUngroupRequest is the body of POST /v1/triage/ungroup. It undoes the
// group the row is in; Revision is the record revision the web read.
type TriageUngroupRequest struct {
	Project  string `json:"project"`
	Revision int    `json:"revision"`
	Address  string `json:"address"`
}

// TriageApproveRequest is the body of POST /v1/triage/approve.
type TriageApproveRequest struct {
	Project  string `json:"project"`
	Revision int    `json:"revision"`
}

// TriageSendBackRequest is the body of POST /v1/triage/send-back.
type TriageSendBackRequest struct {
	Project  string `json:"project"`
	Revision int    `json:"revision"`
}

// TriageRowActionRequest is the body of POST /v1/triage/retry and
// /v1/triage/keep: one failed row at step write.
type TriageRowActionRequest struct {
	Project  string `json:"project"`
	Revision int    `json:"revision"`
	Address  string `json:"address"`
}

// TriageAbandonRequest is the body of POST /v1/triage/abandon.
type TriageAbandonRequest struct {
	Project  string `json:"project"`
	Revision int    `json:"revision"`
}

// TriageEditResult is the result of a row or abandon write. A refusal is a
// result (Refused set); the web reloads on ReasonStale.
type TriageEditResult struct {
	Refused  bool   `json:"refused,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Message  string `json:"message,omitempty"`
	Revision int    `json:"revision,omitempty"`
}

// TriageDestination is one choice for where a row goes.
type TriageDestination struct {
	// Kind is "suggested", "map-epic" or "new-map-epic".
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
	Label string `json:"label"`
}

// TriageResultItem is one row in a result group.
type TriageResultItem struct {
	Address string `json:"address"`
	Title   string `json:"title"`
	// Target is the epic or ticket the row went to; Reason is the close reason.
	Target string `json:"target,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// TriageResultGroup is the done rows of one action. Action is one of ready,
// map, merge, close, keep.
type TriageResultGroup struct {
	Action string             `json:"action"`
	Items  []TriageResultItem `json:"items"`
}

// TriageResult is what a finished Triage did.
type TriageResult struct {
	// ReadyEpic is the epic the ready rows went to.
	ReadyEpic string              `json:"ready_epic,omitempty"`
	Groups    []TriageResultGroup `json:"groups"`
}

// TriageHistoryItem is one past Triage.
type TriageHistoryItem struct {
	ID        string       `json:"id"`
	State     TriageState  `json:"state"`
	StartedAt string       `json:"started_at"`
	Rows      int          `json:"rows"`
	Result    TriageResult `json:"result"`
}

// TriageView is the payload of GET /v1/triage?project=: the live Triage of the
// project, else the latest, plus the past ones.
type TriageView struct {
	Project string      `json:"project"`
	ID      string      `json:"id,omitempty"`
	State   TriageState `json:"state"`
	Flow    string      `json:"flow"`
	Steps   []MapStep   `json:"steps"`
	// StepNote describes progress inside the current step.
	StepNote    string `json:"step_note,omitempty"`
	RingPercent int    `json:"ring_percent"`
	Revision    int    `json:"revision,omitempty"`
	// Round counts the times the answers were sent back to the agent.
	Round int `json:"round,omitempty"`

	Rows          []TriageRow         `json:"rows"`
	OpenQuestions int                 `json:"open_questions"`
	ActionCounts  map[string]int      `json:"action_counts"`
	ChangedRows   int                 `json:"changed_rows"`
	Destinations  []TriageDestination `json:"destinations"`
	// EpicActions lists the actions whose row takes an epic destination.
	EpicActions []string `json:"epic_actions"`
	ReadyEpic   string   `json:"ready_epic,omitempty"`
	// Result is set once the Triage is done.
	Result     *TriageResult       `json:"result,omitempty"`
	CanAbandon bool                `json:"can_abandon"`
	History    []TriageHistoryItem `json:"history"`
}
