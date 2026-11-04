package wire

import (
	"time"
)

// Epic kinds of an EpicDetail.
const (
	EpicKindLoop = "loop"
	EpicKindMap  = "map"
)

// EpicDetailRow is one ticket of an EpicDetail. A map epic's rows leave Parent,
// Cost, Asking, Question and Reason empty.
type EpicDetailRow struct {
	Address        string `json:"address"`
	Title          string `json:"title"`
	RenderedStatus string `json:"rendered_status"`
	Type           string `json:"type,omitempty"`
	Parent         string `json:"parent,omitempty"`
	// LongTicketTurns and LongTicketThreshold feed the Long-running badge; both are 0 when unmarked.
	LongTicketTurns     int `json:"long_ticket_turns,omitempty"`
	LongTicketThreshold int `json:"long_ticket_threshold,omitempty"`
	// Elapsed is in seconds and set once the ticket merges; a running row ticks
	// on the client from ClaimedAt. On a map row it is the done time minus the
	// claim time, absent when either is unset.
	Elapsed int     `json:"elapsed,omitempty"`
	Cost    float64 `json:"cost,omitempty"`
	// HadCodex marks a ticket with a Codex session; its Cost covers only the Claude part.
	HadCodex  bool      `json:"had_codex,omitempty"`
	ClaimedAt time.Time `json:"claimed_at,omitzero"`
	// ClosedAt is when the ticket was done, else cancelled; the page sorts the done group by it.
	ClosedAt time.Time `json:"closed_at,omitzero"`
	// Blockers are the unresolved ones: own-epic bare ("04"), cross-epic as "epic/NN".
	Blockers []string `json:"blockers,omitempty"`
	// Asking is set on a paused ticket with an open question; Question is its summary line.
	Asking   bool   `json:"needs_answer,omitempty"`
	Question string `json:"question,omitempty"`
	// Reason is the plain-words pause reason of a paused loop-epic ticket.
	Reason string `json:"reason,omitempty"`
	// State is the token the fix-agent route needs; set on a row paused for repair.
	State string `json:"state,omitempty"`
	// FixPrompt is the prompt the fix agent starts from; set on a row paused for repair.
	FixPrompt string `json:"fix_prompt,omitempty"`
	// Current and Blocked are set on a map epic's rows: Current is the claimed
	// ticket, else the frontier one; Blocked is an open ticket with unresolved blockers.
	Current bool `json:"current,omitempty"`
	Blocked bool `json:"blocked,omitempty"`
	// HarnessLabel is set on a loop epic's open, blocked and claimed rows.
	HarnessLabel `tstype:",extends"`
}

// MapStepState is where a Progress step stands.
type MapStepState string

const (
	MapStepDone    MapStepState = "done"
	MapStepCurrent MapStepState = "current"
	MapStepTodo    MapStepState = "todo"
)

// MapStep is one of the eight Progress steps of a map epic.
type MapStep struct {
	Step  ProgressStep `json:"step"`
	Label string       `json:"label"`
	State MapStepState `json:"state"`
}

// EpicStats is an epic page's Stats section. Every field is absent when it is
// not stamped, never zero by default.
type EpicStats struct {
	LinesAdded   *int   `json:"lines_added,omitempty"`
	LinesRemoved *int   `json:"lines_removed,omitempty"`
	TotalTokens  *int64 `json:"total_tokens,omitempty"`
	// Cost is absent when any ticket had a Codex session.
	Cost *float64 `json:"cost,omitempty"`
	// LeadTime is in seconds, from created_at to the merge (loop) or completion
	// (map); LeadTimeSoFar is set while that end is unset and LeadTime runs to now.
	LeadTime      *int `json:"lead_time,omitempty"`
	LeadTimeSoFar bool `json:"lead_time_so_far,omitempty"`
	// Friction is a loop epic's summed friction values.
	Friction *EpicFriction `json:"friction,omitempty"`
}

// EpicFriction is the Friction part of EpicStats.
type EpicFriction struct {
	// PausesByReason counts pauses by reason, so the client can show the top ones.
	PausesByReason map[string]int `json:"pauses_by_reason,omitempty"`
	RunEvents      map[string]int `json:"run_events,omitempty"`
	PausedSecs     int            `json:"paused_secs"`
	// WebShare is the fraction of pause endings made on the web; absent with no endings.
	WebShare *float64 `json:"web_share,omitempty"`
}

// EpicDetail is the /v1/epics/detail payload. Seconds and dollars throughout;
// the payload carries no token counts outside Stats.
type EpicDetail struct {
	Address string `json:"address"`
	Kind    string `json:"kind"`
	// State is empty for a map epic.
	State   EpicState `json:"state,omitempty"`
	Summary string    `json:"summary"`
	// Done counts cancelled tickets too.
	Done  int `json:"done"`
	Total int `json:"total"`
	// WallClock runs from the epic's started_at to completed_at, or to now while
	// it runs; it is absent before the first claim. AgentTime and CostTotal add up
	// merged tickets only and are always sent, so a zero total still shows.
	WallClock int             `json:"wall_clock,omitempty"`
	AgentTime int             `json:"agent_time"`
	CostTotal float64         `json:"cost_total"`
	Rows      []EpicDetailRow `json:"rows"`
	// Queueable is set when the Queue action applies; see epicQueueable.
	Queueable bool `json:"queueable"`
	// Queued is set when the epic holds a queue item. A Waiting epic that is not
	// queued is not waiting for a slot.
	Queued bool `json:"queued"`
	// MergeBlockKind and MergeBlockReason describe why a Merge blocked epic's last
	// merge paused.
	MergeBlockKind   string `json:"land_block_kind,omitempty"`
	MergeBlockReason string `json:"land_block_reason,omitempty"`
	// MergeBlockFiles is every file a dirty-trunk pause names; the reason caps its list.
	MergeBlockFiles []string `json:"land_block_files,omitempty"`
	// Actions are the verbs of a Merge blocked epic's pause, from the pause-to-verb
	// table, in display order.
	Actions []string `json:"actions,omitempty"`
	// ActionInfo gives each entry of Actions its button label and tooltip.
	ActionInfo map[string]ActionInfo `json:"action_info,omitempty"`
	// Steps is a map epic's Progress, and ImplAddress the address of its impl epic once it exists.
	Steps       []MapStep `json:"steps,omitempty"`
	ImplAddress string    `json:"impl_address,omitempty"`
	// RingProgress is a map epic's Design progress in percent, as on the Design card.
	RingProgress int `json:"ring_progress,omitempty"`
	// DesignSection is a map epic's own Design card section, which the page's tone
	// reads even when a joined card shows the -impl epic instead.
	DesignSection CardSection `json:"design_section,omitempty"`
	// Draft is set on a loop epic that is still being published into.
	Draft bool `json:"draft,omitempty"`
	// Deletable is set on an uncharted design, the rule the designs delete route guards.
	Deletable bool      `json:"deletable,omitempty"`
	Stats     EpicStats `json:"stats"`
}
