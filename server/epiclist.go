package server

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// EpicListState is EpicListRow.State. It is wider than EpicState: it also
// names the epics the Epics page lists that no queue item holds.
type EpicListState string

const (
	EpicListReady           EpicListState = "ready"
	EpicListDesigning       EpicListState = "designing"
	EpicListWaitingToDesign EpicListState = "waiting-to-design"
	EpicListFinished        EpicListState = "finished"
	EpicListQueued          EpicListState = "queued"
	EpicListRunning         EpicListState = "running"
	EpicListAwaitingMerge   EpicListState = "awaiting-merge"
	EpicListParked          EpicListState = "parked"
	EpicListDeadlocked      EpicListState = "deadlocked"
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
}

// EpicList is the /v1/epics payload.
type EpicList struct {
	Epics []EpicListRow `json:"epics"`
}

// epicListed reports whether the Epics page can show e: one-offs, follow-ups
// and archived epics are never listed.
func epicListed(e tickets.Epic) bool {
	return e.Name != followUpsEpicName && !strings.HasSuffix(e.Name, "-archived") && !IsOneOffEpic(e)
}

// epicListState is the one rule for a listed epic's state. A map has no
// queue state, and a loop epic is draft, finished or whatever the queue and
// its tickets say.
func epicListState(e tickets.Epic, queued bool, loopState EpicState) EpicListState {
	switch {
	case e.IsMap:
		return mapListState(e)
	case !e.MergedAt.IsZero():
		return EpicListFinished
	case e.Status == string(schema.StatusDraft):
		return EpicListDraft
	}
	switch loopState {
	case EpicStateAwaitingMerge:
		return EpicListAwaitingMerge
	case EpicStateRunning:
		return EpicListRunning
	case EpicStateParked:
		return EpicListParked
	case EpicStateDeadlocked:
		return EpicListDeadlocked
	}
	switch {
	case queued:
		return EpicListQueued
	case epicQueueable(e, false):
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
	list := EpicList{Epics: []EpicListRow{}}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		epics, err := s.projectEpics(project)
		if err != nil {
			continue
		}
		for _, e := range epics {
			if epicListed(e) {
				list.Epics = append(list.Epics, s.epicListRow(project, e))
			}
		}
	}
	writeJSON(w, list)
}

func (s *Server) epicListRow(project string, e tickets.Epic) EpicListRow {
	root := rootRef{Project: project, Epic: e.Name}
	queued := s.rootQueued(root)
	row := EpicListRow{
		Address: root.String(), Project: project, Name: e.Name, Kind: EpicKindLoop,
		Done: e.DoneCount(), Total: e.TotalCount(),
		CompletedAt: e.CompletedAt, MergedAt: e.MergedAt, CreatedAt: e.CreatedAt,
		Queued: queued, Queueable: epicQueueable(e, queued),
	}
	if e.IsMap {
		row.Kind = EpicKindMap
	}
	row.State = epicListState(e, queued, epicStateFor(root, e, s.registry.startedAtOf))
	return row
}
