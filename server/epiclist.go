package server

import (
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
	EpicListFinished      = EpicListState(EpicStateFinished)
	EpicListRunning       = EpicListState(EpicStateRunning)
	EpicListAwaitingMerge = EpicListState(EpicStateAwaitingMerge)
	EpicListMerging       = EpicListState(EpicStateMerging)
	EpicListPaused        = EpicListState(EpicStatePaused)

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
		for _, e := range epics {
			merged[e.Name] = !e.MergedAt.IsZero()
			if e.Name == followUpsEpicName {
				if open := openFollowUps(e); open > 0 {
					list.FollowUps = append(list.FollowUps, FollowUpCount{Project: project, Open: open})
				}
			}
		}
		for _, e := range epics {
			if epicListed(e) {
				row := s.epicListRow(project, e)
				row.BlockedByEpics = unmergedBlockers(project, e, merged)
				list.Epics = append(list.Epics, row)
			}
		}
	}
	writeJSON(w, list)
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
		Queued: queued, Queueable: queueable,
		LastActivityAt: lastActivity(e),
	}
	if e.IsMap {
		row.Kind = EpicKindMap
	}
	row.State = epicListState(e, queued, queueable, epicStateFor(root, e, s.registry.startedAtOf))
	if row.State == EpicListMergeBlocked {
		row.MergeBlockKind, row.MergeBlockReason = e.MergeBlockKind, e.MergeBlockReason
	}
	return row
}
