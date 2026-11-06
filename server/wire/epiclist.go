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
	// Verdict is VerdictConcurrencyCap on a queued epic held by a cap; Reason
	// then names the full caps.
	Verdict string `json:"verdict,omitempty"`
	// LaunchStep is the step of the epic's launching ticket; the card shows it
	// instead of the cap line.
	LaunchStep LaunchStep `json:"launch_step,omitzero"`
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

// CardSection is the Epics page section a card belongs to. The server decides it;
// the web client does not derive sections.
type CardSection string

// Spelled as literals for tygo; see the comment on EpicListState.
const (
	SectionNeedsYou      CardSection = "needs-you"
	SectionImplementing  CardSection = "implementing"
	SectionDesigning     CardSection = "designing"
	SectionReadyToQueue  CardSection = "ready-to-queue"
	SectionReadyToDesign CardSection = "ready-to-design"
	SectionFinished      CardSection = "finished"
)

// CardKind is the kind of an Epics page card.
type CardKind string

const (
	CardKindImplementation CardKind = "implementation"
	CardKindDesign         CardKind = "design"
	CardKindOneOff         CardKind = "one-off"
	CardKindFollowUps      CardKind = "follow-ups"
)

// CardActionKind names what a card button does.
type CardActionKind string

const (
	CardActionQueue      CardActionKind = "queue"
	CardActionAnswer     CardActionKind = "answer"
	CardActionOpenTicket CardActionKind = "open-ticket"
	CardActionOpenEpic   CardActionKind = "open-epic"
	CardActionMerge      CardActionKind = "merge"
	// StashMerge targets the epic like Merge; its route is a later ticket's.
	CardActionStashMerge CardActionKind = "stash-merge"
	// The Design actions target the Design's epic address. Design and Continue
	// start a session first (POST open-design); Answer and Open only navigate.
	CardActionDesign   CardActionKind = "design"
	CardActionContinue CardActionKind = "continue"
	CardActionOpen     CardActionKind = "open"
	// Triage targets the project whose follow-ups the card counts.
	CardActionTriage CardActionKind = "triage"
)

// CardAction is one button of a card. Target is the address the button opens (a
// ticket for Answer and Open ticket, an epic for Open epic) or acts on (Merge).
type CardAction struct {
	Kind   CardActionKind `json:"kind"`
	Label  string         `json:"label"`
	Target string         `json:"target,omitempty"`
	// Link is the hash route the button opens, when the server decides it. Answer
	// sets it for a Design question; the client falls back to the ticket page.
	Link string `json:"link,omitempty"`
}

// CardTimeKind says what the card's start time measures; the client words it.
type CardTimeKind string

// Spelled as literals for tygo; see the comment on EpicListState.
const (
	CardTimeElapsed  CardTimeKind = "elapsed"
	CardTimeWaiting  CardTimeKind = "waiting"
	CardTimeCreated  CardTimeKind = "created"
	CardTimeFinished CardTimeKind = "finished"
)

// EpicState is EpicSummary.State.
type EpicState string

const (
	EpicStateAwaitingMerge EpicState = "awaiting-merge"
	EpicStateFinished      EpicState = "finished"
	EpicStateRunning       EpicState = "running"
	// EpicStateMerging is an epic whose only unfinished tickets are merge tickets
	// that are open or claimed.
	EpicStateMerging EpicState = "merging"
	EpicStateWaiting EpicState = "waiting"
	// EpicStatePaused is an epic with nothing runnable: a paused ticket, a
	// dependency deadlock, or a blocked merge. EpicSummary.Reason says which.
	EpicStatePaused EpicState = "paused"
)

// The reasons a Paused epic gives for itself, when no ticket is paused.
const (
	EpicReasonDeadlock     = "Dependency deadlock"
	EpicReasonMergeBlocked = "Merge blocked"
)

// SummaryTicket is one ticket in an EpicSummary: an active one carries
// ClaimedAt, a Paused one carries Recovery and its unclipped Pause reason.
type SummaryTicket struct {
	Address   string    `json:"address"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	ClaimedAt time.Time `json:"claimed_at,omitzero"`
	// LaunchStep is set while an active ticket's launch runs.
	LaunchStep LaunchStep `json:"launch_step,omitzero"`
	Recovery   string     `json:"recovery,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	// ReasonLabel is the plain-words label of the ticket's pause reason.
	ReasonLabel string `json:"reason_label,omitempty"`
	// Summary is the open question's summary line, or the pause reason without one.
	Summary string `json:"summary,omitempty"`
	// Answerable is set when a person's reply can go to the open question.
	Answerable bool `json:"answerable"`
	// Answered is set on an open ticket whose latest question has an answer.
	Answered bool `json:"answered"`
	// NeedsApproval is set on an active ticket whose agent waits on a permission prompt.
	NeedsApproval bool `json:"needs_approval"`
}

// EpicSummary is one epic holding a queue item, as the web page's cards show it.
type EpicSummary struct {
	Address string    `json:"address"`
	Name    string    `json:"name"`
	Project string    `json:"project"`
	State   EpicState `json:"state"`
	Done    int       `json:"done"`
	Total   int       `json:"total"`
	// Active is the running tickets, listed only while the epic is Running.
	Active []SummaryTicket `json:"active"`
	Paused []SummaryTicket `json:"parked"`
	// HasRepair is set when a paused ticket is broken (stuck or error), so
	// both clients show a fault in the alarm color instead of a question.
	HasRepair bool `json:"has_repair"`
	// Reason is why a Paused epic is paused: the reason of its first paused
	// ticket, else "Dependency deadlock" or "Merge blocked". It is empty for a
	// Paused epic that holds only drafts.
	Reason string `json:"reason,omitempty"`
	// Verdict and VerdictReason are the next ready ticket's cap verdict while a
	// cap holds the epic: VerdictConcurrencyCap and the full caps.
	Verdict       string `json:"verdict,omitempty"`
	VerdictReason string `json:"verdict_reason,omitempty"`
	// More is the number of paused tickets beyond the first, whose reason Reason shows.
	More int `json:"more,omitempty"`
	// Detail is the cycle that deadlocks the epic, or why its merge is blocked.
	Detail string `json:"detail,omitempty"`
	// MergeBlocked is set on a Paused epic whose last merge failed, so clients
	// keep offering the merge action.
	MergeBlocked bool `json:"merge_blocked,omitempty"`
	// MergeBlockKind is the pause reason of a blocked merge.
	MergeBlockKind string `json:"merge_block_kind,omitempty"`
	// Actions are the verbs of a blocked merge's pause, from the pause-to-verb table.
	Actions []string `json:"actions,omitempty"`
}
