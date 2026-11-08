package wire

// EventTriageChanged is emitted with the project name as its address when a
// project's live Triage record changed.
const EventTriageChanged = "triage-changed"

// TriageChangedRequest is the body of POST /v1/triage/changed: a CLI verb's
// "this project's Triage record changed" ping.
type TriageChangedRequest struct {
	Project string `json:"project"`
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
	// Answers maps a question id to the person's answer.
	Answers map[string]string `json:"answers,omitempty"`
	// Action and Destination are the plan's, replaced by the person's when set.
	Action        string `json:"action,omitempty"`
	Destination   string `json:"destination,omitempty"`
	PersonChanged bool   `json:"person_changed,omitempty"`
	Gone          bool   `json:"gone,omitempty"`
	WriteState    string `json:"write_state,omitempty"`
	NewAddress    string `json:"new_address,omitempty"`
	Refusal       string `json:"refusal,omitempty"`
	CanKeep       bool   `json:"can_keep,omitempty"`
	CanRetry      bool   `json:"can_retry,omitempty"`
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

	Rows          []TriageRow         `json:"rows"`
	OpenQuestions int                 `json:"open_questions"`
	ActionCounts  map[string]int      `json:"action_counts"`
	ChangedRows   int                 `json:"changed_rows"`
	Destinations  []TriageDestination `json:"destinations"`
	ReadyEpic     string              `json:"ready_epic,omitempty"`
	// Result is set once the Triage is done.
	Result     *TriageResult       `json:"result,omitempty"`
	CanAbandon bool                `json:"can_abandon"`
	History    []TriageHistoryItem `json:"history"`
}
