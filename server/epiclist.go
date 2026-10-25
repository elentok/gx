package server

import (
	"cmp"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
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
)

// ProgressStep is the step a map's design has reached; see mapProgress. The
// tickets package owns the list of steps.
type ProgressStep = tickets.DesignStep

const (
	ProgressChart          = tickets.StepChart
	ProgressResolveTickets = tickets.StepResolveTickets
	ProgressSpec           = tickets.StepSpec
	ProgressReview         = tickets.StepReview
	ProgressGrillFindings  = tickets.StepGrillFindings
	ProgressCreateTickets  = tickets.StepCreateTickets
	ProgressBranch         = tickets.StepBranch
	ProgressDone           = tickets.StepDone
)

// PickerSection is the Designs picker section a map row belongs to.
type PickerSection string

const (
	PickerInProgress       PickerSection = "in-progress"
	PickerNotStarted       PickerSection = "not-started"
	PickerReadyToQueue     PickerSection = "ready-to-queue"
	PickerRecentlyFinished PickerSection = "recently-finished"
	// PickerNone: finished more than 7 days ago; the row stays on the Epics page.
	PickerNone PickerSection = "none"
)

// followUpsEpicName is every project's contributor outbox epic.
const followUpsEpicName = "follow-ups"

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
	// PickerSection, PickerSortAt and ImplAddress are set only on a map row.
	// PickerSortAt is the time the section sorts by; ImplAddress is the
	// implementation epic's address when it exists.
	PickerSection PickerSection `json:"picker_section,omitempty"`
	PickerSortAt  time.Time     `json:"picker_sort_at,omitzero"`
	ImplAddress   string        `json:"impl_address,omitempty"`

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
	// FollowUps lists only the projects with at least one open follow-up.
	FollowUps []FollowUpCount `json:"follow_ups"`
}

// epicListed reports whether the Epics page can show e: one-offs, follow-ups
// and archived epics are never listed.
func epicListed(e tickets.Epic) bool {
	return e.Name != followUpsEpicName && !strings.HasSuffix(e.Name, "-archived") && !IsOneOffEpic(e)
}

// epicListState is the one rule for a listed epic's state. A map has no
// queue state, and a loop epic is draft, finished or whatever the queue and
// its tickets say.
func epicListState(e tickets.Epic, queued, queueable bool, loopState EpicState) EpicListState {
	switch {
	case e.IsMap:
		return mapListState(e)
	case !e.MergedAt.IsZero():
		return EpicListFinished
	case e.Status == string(schema.StatusDraft):
		return EpicListDraft
	case loopState == EpicStatePaused && epicMergeBlocked(e):
		return EpicListMergeBlocked
	case loopState != EpicStateWaiting && loopState != EpicStateFinished:
		return EpicListState(loopState)
	case queued:
		return EpicListQueued
	case queueable:
		return EpicListReady
	}
	return EpicListNoOpenTickets
}

// mapListState: a map with a completed_at is finished. Otherwise it is
// designing once a ticket is claimed or done, and waiting to design before.
func mapListState(e tickets.Epic) EpicListState {
	switch {
	case !e.CompletedAt.IsZero():
		return EpicListFinished
	case slices.ContainsFunc(e.Tickets, func(t tickets.Ticket) bool {
		return t.Status == string(schema.StatusClaimed) || t.Status == string(schema.StatusDone)
	}):
		return EpicListDesigning
	}
	return EpicListWaitingToDesign
}

func (s *Server) epicList(w http.ResponseWriter, _ *http.Request) {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	list := EpicList{Epics: []EpicListRow{}, FollowUps: []FollowUpCount{}}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		epics, err := s.projectEpics(project)
		if err != nil {
			continue
		}
		merged := map[string]bool{}
		byName := map[string]tickets.Epic{}
		hasMap := false
		for _, e := range epics {
			merged[e.Name] = !e.MergedAt.IsZero()
			byName[e.Name] = e
			hasMap = hasMap || (e.IsMap && epicListed(e))
			if e.Name == followUpsEpicName {
				if open := openFollowUps(e); open > 0 {
					list.FollowUps = append(list.FollowUps, FollowUpCount{Project: project, Open: open})
				}
			}
		}
		var designs map[string]designActivity
		if hasMap {
			designs = s.designActivities()
		}
		now := time.Now()
		for _, e := range epics {
			if epicListed(e) {
				row := s.epicListRow(project, e)
				row.BlockedByEpics = unmergedBlockers(project, e, merged)
				if e.IsMap {
					s.decorateMapRow(&row, project, e, byName, designs[row.Address], now)
				}
				list.Epics = append(list.Epics, row)
			}
		}
	}
	sortCards(list.Epics, func(r EpicListRow) int {
		return s.rootQueuePosition(rootRef{Project: r.Project, Epic: r.Name})
	})
	writeJSON(w, list)
}

// decorateMapRow adds the map-only fields to a listed map epic's row.
func (s *Server) decorateMapRow(row *EpicListRow, project string, e tickets.Epic, byName map[string]tickets.Epic, design designActivity, now time.Time) {
	row.ProgressStep = mapProgress(e)
	row.PickerSection, row.PickerSortAt, row.ImplAddress = s.mapPicker(project, e, row.ProgressStep, byName, design, now)
}

// openFollowUps counts the draft and open tickets of a follow-ups epic.
func openFollowUps(e tickets.Epic) int {
	n := 0
	for _, t := range e.Tickets {
		if t.Status == string(schema.StatusDraft) || t.Status == string(schema.StatusOpen) {
			n++
		}
	}
	return n
}

// unmergedBlockers returns the addresses of the epics in e's blocked_by that
// exist in the project and have not merged. Ticket refs are not epic blockers.
func unmergedBlockers(project string, e tickets.Epic, merged map[string]bool) []string {
	out := []string{}
	for _, ref := range e.BlockedBy {
		p, name, isEpic := tickets.BlockedByEpicRef(project, ref)
		if !isEpic || p != project {
			continue
		}
		if isMerged, exists := merged[name]; exists && !isMerged {
			out = append(out, rootRef{Project: project, Epic: name}.String())
		}
	}
	return out
}

// lastActivity is the newest claimed_at or done_at across e's tickets.
func lastActivity(e tickets.Epic) time.Time {
	var newest time.Time
	for _, t := range e.Tickets {
		for _, at := range []time.Time{t.ClaimedAt, t.DoneAt} {
			if at.After(newest) {
				newest = at
			}
		}
	}
	return newest
}

func (s *Server) epicListRow(project string, e tickets.Epic) EpicListRow {
	root := rootRef{Project: project, Epic: e.Name}
	queued := s.rootQueued(root)
	queueable := epicQueueable(e, queued)
	row := EpicListRow{
		Address: root.String(), Project: project, Name: e.Name, Kind: EpicKindLoop,
		Done: e.DoneCount(), Total: e.TotalCount(),
		CompletedAt: e.CompletedAt, MergedAt: e.MergedAt, CreatedAt: e.CreatedAt,
		QueuedAt: cmp.Or(e.QueuedAt, e.StartedAt, e.MergedAt),
		Queued:   queued, Queueable: queueable,
		LastActivityAt: lastActivity(e),
	}
	if e.IsMap {
		row.Kind = EpicKindMap
	}
	row.State = epicListState(e, queued, queueable, epicStateFor(root, e, s.registry.startedAtOf))
	if row.State == EpicListMergeBlocked {
		row.MergeBlockKind, row.MergeBlockReason = e.MergeBlockKind, e.MergeBlockReason
	}
	if !e.IsMap {
		decorateLoopCard(&row, s.rootQueuePosition(root))
		if n, ok := s.loopNeedsYou(root, e, row.State); ok {
			row.Section, row.StatePill, row.Reason, row.WaitingSince = SectionNeedsYou, n.Pill, n.Reason, n.Since
			row.Actions = []CardAction{n.Action}
		}
	}
	return row
}
