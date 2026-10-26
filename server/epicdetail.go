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

// EpicDetailRow is one ticket of an EpicDetail. A map epic's rows leave Parent,
// Cost, Asking, Question and Reason empty.
type EpicDetailRow struct {
	Address        string `json:"address"`
	Title          string `json:"title"`
	RenderedStatus string `json:"rendered_status"`
	Type           string `json:"type,omitempty"`
	Parent         string `json:"parent,omitempty"`
	// Elapsed is in seconds and set once the ticket merges; a running row ticks
	// on the client from ClaimedAt. On a map row it is the done time minus the
	// claim time, absent when either is unset.
	Elapsed   int       `json:"elapsed,omitempty"`
	Cost      float64   `json:"cost,omitempty"`
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
	// Current and Blocked are set on a map epic's rows: Current is the claimed
	// ticket, else the frontier one; Blocked is an open ticket with unresolved blockers.
	Current bool `json:"current,omitempty"`
	Blocked bool `json:"blocked,omitempty"`
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
	// merge paused; the kind is empty for a needs-rebase pause.
	MergeBlockKind   string `json:"land_block_kind,omitempty"`
	MergeBlockReason string `json:"land_block_reason,omitempty"`
	// Steps is a map epic's Progress, and ImplAddress the address of its impl epic once it exists.
	Steps       []MapStep `json:"steps,omitempty"`
	ImplAddress string    `json:"impl_address,omitempty"`
	// RingProgress is a map epic's Design progress in percent, as on the Design card.
	RingProgress int `json:"ring_progress,omitempty"`
	// Draft is set on a loop epic that is still being published into.
	Draft bool `json:"draft,omitempty"`
	// Deletable is set on an uncharted design, the rule the designs delete route guards.
	Deletable bool      `json:"deletable,omitempty"`
	Stats     EpicStats `json:"stats"`
}

// epicQueueable reports whether an epic can be queued: an unqueued, unmerged
// loop epic that still has an open ticket. The epic list shares this rule.
func epicQueueable(e tickets.Epic, queued bool) bool {
	if e.IsMap || queued || !e.MergedAt.IsZero() || e.QueueRefusal() != tickets.QueueRefusalNone {
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
	writeJSON(w, s.buildEpicDetail(root, epics, epics[i], time.Now()))
}

func (s *Server) buildEpicDetail(root rootRef, epics []tickets.Epic, e tickets.Epic, now time.Time) EpicDetail {
	if e.IsMap {
		return s.buildMapDetail(root, epics, e, now)
	}
	return s.buildLoopDetail(root, e, now)
}

func (s *Server) buildMapDetail(root rootRef, epics []tickets.Epic, e tickets.Epic, now time.Time) EpicDetail {
	byName := map[string]tickets.Epic{}
	for _, other := range epics {
		byName[other.Name] = other
	}
	queued := s.rootQueued(root)
	d := EpicDetail{
		Address: root.String(), Kind: EpicKindMap, Summary: schema.Section(e.Body, "Destination"),
		Done: e.DoneCount(), Total: e.TotalCount(), Rows: []EpicDetailRow{},
		Queueable: epicQueueable(e, queued), Queued: queued,
	}
	if deletable, err := s.designUncharted(root.String(), e); err != nil {
		s.log.Warn("design deletable check", "epic", root.String(), "err", err)
	} else {
		d.Deletable = deletable
	}
	d.Steps = mapSteps(e, byName)
	if step := mapProgress(e); step == ProgressDone {
		d.RingProgress = 100
	} else {
		d.RingProgress = designPercent(e, step)
	}
	d.Stats = epicStats(e, e.CompletedAt, now)
	if _, ok := byName[e.Name+tickets.ImplEpicSuffix]; ok {
		d.ImplAddress = rootRef{Project: root.Project, Epic: e.Name + tickets.ImplEpicSuffix}.String()
	}
	current, blocked := mapTicketFlags(e)
	for _, t := range e.Tickets {
		status := e.RenderedStatus(t)
		row := EpicDetailRow{
			Address: ticketAddress(root, t.DisplayNumber()), Title: t.Title,
			RenderedStatus: status.Word(), Type: t.Type, Blockers: e.UnresolvedBlockers(t),
			ClosedAt: t.FinishedAt(),
			Current:  current.DisplayNumber() != "" && t.DisplayNumber() == current.DisplayNumber(),
			Blocked:  blocked[t.DisplayNumber()],
		}
		if status == tickets.StatusClaimed {
			row.ClaimedAt = t.ClaimedAt
		}
		if !t.ClaimedAt.IsZero() && !t.DoneAt.IsZero() {
			row.Elapsed = int(t.DoneAt.Sub(t.ClaimedAt).Seconds())
		}
		d.Rows = append(d.Rows, row)
	}
	return d
}

func (s *Server) buildLoopDetail(root rootRef, e tickets.Epic, now time.Time) EpicDetail {
	queued := s.rootQueued(root)
	d := EpicDetail{
		Address: root.String(), Kind: EpicKindLoop, Summary: schema.Section(e.Body, "Summary"),
		State: epicStateFor(root, e, s.registry.startedAtOf),
		Done:  e.DoneCount(), Total: e.TotalCount(), Rows: []EpicDetailRow{},
		Queueable: epicQueueable(e, queued), Queued: queued,
		Draft: e.Status == string(schema.StatusDraft),
	}
	if epicMergeBlocked(e) {
		d.MergeBlockKind, d.MergeBlockReason = e.MergeBlockKind, e.MergeBlockReason
	}
	for _, t := range e.Tickets {
		d.Rows = append(d.Rows, s.rowFor(root, e, t))
		if t.Status == string(schema.StatusDone) {
			d.AgentTime += t.ElapsedTime
			d.CostTotal += t.ActualCost
		}
	}
	if !e.StartedAt.IsZero() {
		end := e.CompletedAt
		if end.IsZero() {
			end = now
		}
		d.WallClock = int(end.Sub(e.StartedAt).Seconds())
	}
	d.Stats = epicStats(e, e.MergedAt, now)
	d.Stats.Friction = epicFriction(e.Friction)
	d.Stats.LinesAdded, d.Stats.LinesRemoved = e.LinesAdded, e.LinesRemoved
	return d
}

// epicStats fills the stamps both epic kinds share. end is when the lead time
// stops: the merge for a loop epic, the completion for a map epic.
func epicStats(e tickets.Epic, end, now time.Time) EpicStats {
	var st EpicStats
	st.TotalTokens = e.TotalTokens
	if e.TotalTokens != nil && !e.HadCodex {
		cost := e.ActualCost
		st.Cost = &cost
	}
	if !e.CreatedAt.IsZero() {
		st.LeadTimeSoFar = end.IsZero()
		if st.LeadTimeSoFar {
			end = now
		}
		lead := int(end.Sub(e.CreatedAt).Seconds())
		st.LeadTime = &lead
	}
	return st
}

func epicFriction(f *schema.FrictionSummary) *EpicFriction {
	if f == nil {
		return nil
	}
	out := &EpicFriction{PausesByReason: f.PausesByReason, RunEvents: f.RunEvents, PausedSecs: f.TotalPausedSecs}
	out.WebShare = webShare(f.PauseEndings)
	return out
}

// webShare is web / (web + TUI + CLI) over pause endings by source; gx- and
// unknown-ended pauses are not person endings, so with none it returns nil.
func webShare(endings map[string]int) *float64 {
	web := endings[tickets.SourceWeb]
	person := web + endings[tickets.SourceTUI] + endings[tickets.SourceCLI]
	if person == 0 {
		return nil
	}
	share := float64(web) / float64(person)
	return &share
}

// rowFor builds a loop epic's full row for t.
func (s *Server) rowFor(root rootRef, e tickets.Epic, t tickets.Ticket) EpicDetailRow {
	status := e.RenderedStatus(t)
	row := EpicDetailRow{
		Address: ticketAddress(root, t.DisplayNumber()), Title: t.Title, RenderedStatus: status.Word(),
		Type: t.Type, Elapsed: t.ElapsedTime, Cost: t.ActualCost, Blockers: e.UnresolvedBlockers(t),
		ClosedAt: t.FinishedAt(),
	}
	if t.Parent != nil {
		row.Parent = ticketAddress(root, *t.Parent)
	}
	if at, ok := s.registry.startedAtOf(row.Address); ok && status == tickets.StatusClaimed {
		row.ClaimedAt = at
	}
	if open := tickets.OpenQuestion(readLogLenient(t)); open != nil && status == tickets.StatusPausedAnswer {
		row.Asking, row.Question = true, open.Summary
	}
	if t.PausedClass() != "" {
		row.Reason = t.PauseReason.Label()
	}
	return row
}

func ticketAddress(root rootRef, id string) string {
	return tickets.Address{Project: root.Project, Epic: root.Epic, ID: id}.String()
}
