package wire

import (
	"github.com/elentok/gx/tickets"
	"time"
)

// EpicListState is EpicListRow.State. It is a superset of EpicState: it also
// names the epics the Epics page lists that no queue item holds.
type EpicListState string

const (
	// Spelled as literals, not EpicListState(EpicState...): tygo emits a conversion as `any`,
	// which throws when the web app imports the generated file.
	EpicListFinished      EpicListState = "finished"
	EpicListRunning       EpicListState = "running"
	EpicListAwaitingMerge EpicListState = "awaiting-merge"
	EpicListMerging       EpicListState = "merging"
	EpicListPaused        EpicListState = "paused"

	EpicListReady           EpicListState = "ready"
	EpicListDesigning       EpicListState = "designing"
	EpicListWaitingToDesign EpicListState = "waiting-to-design"
	EpicListQueued          EpicListState = "queued"
	EpicListMergeBlocked    EpicListState = "merge-blocked"
	EpicListDraft           EpicListState = "draft"
	// EpicListNoOpenTickets is an open, unqueued loop epic with no open ticket:
	// nothing to queue, yet not finished.
	EpicListNoOpenTickets EpicListState = "no-open-tickets"
	// EpicListFollowUps is a project's follow-ups card; see followUpsCard.
	EpicListFollowUps EpicListState = "follow-ups"
)

// ProgressStep is the step a map's design has reached; see mapProgress. The
// tickets package owns the list of steps. The constants below are literals,
// not tickets.Step*, because tygo cannot resolve another package's constant;
// TestProgressStepsMatchTickets keeps them in sync.
type ProgressStep = tickets.DesignStep

const (
	ProgressChart          ProgressStep = "chart"
	ProgressResolveTickets ProgressStep = "resolve-tickets"
	ProgressSpec           ProgressStep = "spec"
	ProgressReview         ProgressStep = "review"
	ProgressGrillFindings  ProgressStep = "grill-findings"
	ProgressCreateTickets  ProgressStep = "create-tickets"
	ProgressBranch         ProgressStep = "branch"
	ProgressDone           ProgressStep = "done"
)

// EpicListRow is one epic of the /v1/epics payload.
type EpicListRow struct {
	Address     string        `json:"address"`
	Project     string        `json:"project"`
	Name        string        `json:"name"`
	Kind        string        `json:"kind"`
	State       EpicListState `json:"state"`
	Done        int           `json:"done"`
	Total       int           `json:"total"`
	CompletedAt time.Time     `json:"completed_at,omitzero"`
	MergedAt    time.Time     `json:"merged_at,omitzero"`
	CreatedAt   time.Time     `json:"created_at,omitzero"`
	// QueuedAt is the epic's first enqueue, else started_at, else merged_at.
	QueuedAt time.Time `json:"queued_at,omitzero"`
	// Queued is set while a queue item holds the epic.
	Queued bool `json:"queued"`
	// Queueable is set when the Queue action applies; see epicQueueable.
	Queueable bool `json:"queueable"`
	// BlockedByEpics are the addresses of the epics named in blocked_by that
	// have not merged yet.
	BlockedByEpics []string `json:"blocked_by_epics"`
	// LastActivityAt is the newest claimed_at or done_at of the epic's tickets.
	LastActivityAt time.Time `json:"last_activity_at,omitzero"`
	// MergeBlockKind and MergeBlockReason are set only on a Merge blocked row;
	// see EpicDetail.
	MergeBlockKind   string `json:"land_block_kind,omitempty"`
	MergeBlockReason string `json:"land_block_reason,omitempty"`
	// ProgressStep is set only on a map row.
	ProgressStep ProgressStep `json:"progress_step,omitempty"`
	// QueueSlot is the 1-based slot of the row's loop epic in the queue, 0 when it
	// is not queued. A joined card carries its -impl epic's slot.
	QueueSlot int `json:"queue_slot,omitempty"`

	// The card fields below are set on loop epic rows. A joined card carries both
	// of its epics' addresses in Addresses, the active part first.
	Addresses []string `json:"addresses,omitempty"`
	CardKind  CardKind `json:"card_kind,omitempty"`
	// Section is where the Epics page puts the card.
	Section     CardSection `json:"section,omitempty"`
	StatusWords string      `json:"status_words,omitempty"`
	// RingProgress is the ring fill in percent, 0 to 100.
	RingProgress int `json:"ring_progress,omitempty"`
	// Reason is the line under the status words when there is something to say.
	Reason  string       `json:"reason,omitempty"`
	Actions []CardAction `json:"actions,omitempty"`
	// StatePill and WaitingSince are set on a Needs-you card: the pill word and the
	// start of the wait the section sorts by.
	StatePill    string    `json:"state_pill,omitempty"`
	WaitingSince time.Time `json:"waiting_since,omitzero"`
	// CountdownEndsAt is the deadline of a Design's auto-advance countdown; the client
	// builds "Next run starts in Ns" from it.
	CountdownEndsAt time.Time `json:"countdown_ends_at,omitzero"`
	// TimeKind and TimeStart are the card's time label: what it measures and when it
	// starts. Both are empty when the start is unknown.
	TimeKind  CardTimeKind `json:"time_kind,omitempty"`
	TimeStart time.Time    `json:"time_start,omitzero"`
}

// FollowUpCount is the number of draft and open tickets in one project's
// follow-ups epic.
type FollowUpCount struct {
	Project string `json:"project"`
	Open    int    `json:"open"`
}

// EpicList is the /v1/epics payload.
type EpicList struct {
	Epics []EpicListRow `json:"epics"`
	// Projects is every project, including those with no epics: the project
	// pickers list them all.
	Projects []string `json:"projects,omitempty"`
	// FollowUps lists only the projects with at least one open follow-up.
	FollowUps []FollowUpCount `json:"follow_ups"`
	// RecentlyFinished is config server.recently-finished: how many Finished cards
	// the page shows before a filter.
	RecentlyFinished int `json:"recently_finished,omitempty"`
}
