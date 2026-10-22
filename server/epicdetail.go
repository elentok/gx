package server

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Epic kinds of an EpicDetail.
const (
	EpicKindLoop = "loop"
	EpicKindMap  = "map"
)

// EpicDetailRow is one ticket of an EpicDetail. A map epic's rows fill only
// Address, Title and RenderedStatus.
type EpicDetailRow struct {
	Address        string `json:"address"`
	Title          string `json:"title"`
	RenderedStatus string `json:"rendered_status"`
	Type           string `json:"type,omitempty"`
	Parent         string `json:"parent,omitempty"`
	// Elapsed is in seconds and set once the ticket lands; a running row ticks
	// on the client from ClaimedAt.
	Elapsed   int       `json:"elapsed,omitempty"`
	Cost      float64   `json:"cost,omitempty"`
	ClaimedAt time.Time `json:"claimed_at,omitzero"`
	// Blockers are the unresolved ones: own-epic bare ("04"), cross-epic as "epic/NN".
	Blockers []string `json:"blockers,omitempty"`
	// NeedsAnswer is set on a parked ticket with an open question; Question is its summary line.
	NeedsAnswer bool   `json:"needs_answer,omitempty"`
	Question    string `json:"question,omitempty"`
}

// EpicDetail is the /v1/epics/detail payload. Seconds and dollars throughout;
// the payload carries no token counts.
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
	// landed tickets only and are always sent, so a zero total still shows.
	WallClock int             `json:"wall_clock,omitempty"`
	AgentTime int             `json:"agent_time"`
	CostTotal float64         `json:"cost_total"`
	Rows      []EpicDetailRow `json:"rows"`
	// Queueable is set when the Queue action applies; see epicQueueable.
	Queueable bool `json:"queueable"`
	// Queued is set when the epic holds a queue item. A Waiting epic that is not
	// queued is not waiting for a slot.
	Queued bool `json:"queued"`
}

// epicQueueable reports whether an epic can be queued: an unqueued, unmerged
// loop epic that still has an open ticket. The epic list shares this rule.
func epicQueueable(e tickets.Epic, queued bool) bool {
	if e.IsMap || queued || !e.MergedAt.IsZero() {
		return false
	}
	return slices.ContainsFunc(e.Tickets, func(t tickets.Ticket) bool { return t.Status == string(schema.StatusOpen) })
}

func (s *Server) epicDetail(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	root, err := parseRootRef(address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	epics, err := s.projectEpics(root.Project)
	var addrErr *tickets.AddressError
	if errors.As(err, &addrErr) {
		http.Error(w, "no epic "+address, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == root.Epic })
	if i < 0 {
		http.Error(w, "no epic "+address, http.StatusNotFound)
		return
	}
	writeJSON(w, s.buildEpicDetail(root, epics[i], time.Now()))
}

func (s *Server) buildEpicDetail(root rootRef, e tickets.Epic, now time.Time) EpicDetail {
	queued := s.rootQueued(root)
	d := EpicDetail{
		Address: root.String(), Kind: EpicKindLoop, Summary: schema.Section(e.Body, "Summary"),
		Done: e.DoneCount(), Total: e.TotalCount(), Rows: []EpicDetailRow{},
		Queueable: epicQueueable(e, queued), Queued: queued,
	}
	if e.IsMap {
		d.Kind, d.Summary = EpicKindMap, schema.Section(e.Body, "Destination")
	} else {
		d.State = epicStateFor(root, e, s.registry.startedAtOf)
	}
	addr := func(id string) string {
		return tickets.Address{Project: root.Project, Epic: root.Epic, ID: id}.String()
	}
	for _, t := range e.Tickets {
		status := e.RenderedStatus(t)
		row := EpicDetailRow{Address: addr(t.DisplayNumber()), Title: t.Title, RenderedStatus: status.Word()}
		if e.IsMap {
			d.Rows = append(d.Rows, row)
			continue
		}
		row.Type, row.Elapsed, row.Cost = t.Type, t.ElapsedTime, t.ActualCost
		row.Blockers = e.UnresolvedBlockers(t)
		if t.Parent != nil {
			row.Parent = addr(*t.Parent)
		}
		if at, ok := s.registry.startedAtOf(row.Address); ok && status == tickets.StatusClaimed {
			row.ClaimedAt = at
		}
		if open := tickets.OpenQuestion(readLogLenient(t)); open != nil && status == tickets.StatusNeedsAnswer {
			row.NeedsAnswer, row.Question = true, open.Summary
		}
		if t.Status == string(schema.StatusDone) {
			d.AgentTime += t.ElapsedTime
			d.CostTotal += t.ActualCost
		}
		d.Rows = append(d.Rows, row)
	}
	if !e.IsMap && !e.StartedAt.IsZero() {
		end := e.CompletedAt
		if end.IsZero() {
			end = now
		}
		d.WallClock = int(end.Sub(e.StartedAt).Seconds())
	}
	return d
}
