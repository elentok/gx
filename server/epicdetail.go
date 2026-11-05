package server

import (
	"errors"
	"github.com/elentok/gx/server/sessions"
	"net/http"
	"slices"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/stats"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// mergeBlockActions is the verbs the pause-to-verb table lists for a blocked
// merge's kind.
func mergeBlockActions(kind string) []string {
	row, _ := schema.PauseReason(kind).Row()
	actions := make([]string, len(row.Verbs))
	for i, v := range row.Verbs {
		actions[i] = v.Word()
	}
	return actions
}

func (s *Server) epicDetail(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	root, err := queue.ParseRootRef(address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	epics, err := s.projects.Epics(root.Project)
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

func (s *Server) buildEpicDetail(root queue.RootRef, epics []tickets.Epic, e tickets.Epic, now time.Time) wire.EpicDetail {
	if e.IsMap {
		return s.buildMapDetail(root, epics, e, now)
	}
	return s.buildLoopDetail(root, e, now)
}

func (s *Server) buildMapDetail(root queue.RootRef, epics []tickets.Epic, e tickets.Epic, now time.Time) wire.EpicDetail {
	byName := map[string]tickets.Epic{}
	for _, other := range epics {
		byName[other.Name] = other
	}
	queued := s.queue.RootQueued(root)
	d := wire.EpicDetail{
		Address: root.String(), Kind: wire.EpicKindMap, Summary: schema.Section(e.Body, "Destination"),
		Done: e.DoneCount(), Total: e.TotalCount(), Rows: []wire.EpicDetailRow{},
		Queueable: queue.EpicQueueable(e, queued), Queued: queued,
	}
	if deletable, err := s.designUncharted(root.String(), e); err != nil {
		s.log.Warn("design deletable check", "epic", root.String(), "err", err)
	} else {
		d.Deletable = deletable
	}
	d.Steps = mapSteps(e, byName)
	step := queue.MapProgress(e)
	d.DesignSection = designSection(d.Address, e, step, s.designActivities()[d.Address])
	d.RingProgress = designPercent(e, step)
	d.Stats = epicStats(e, e.CompletedAt, now)
	if _, ok := byName[e.Name+tickets.ImplEpicSuffix]; ok {
		d.ImplAddress = queue.RootRef{Project: root.Project, Epic: e.Name + tickets.ImplEpicSuffix}.String()
	}
	current, blocked := mapTicketFlags(e)
	for _, t := range e.Tickets {
		status := e.RenderedStatus(t)
		row := baseRow(root, e, t, status)
		row.Current = current.DisplayNumber() != "" && t.DisplayNumber() == current.DisplayNumber()
		row.Blocked = blocked[t.DisplayNumber()]
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

func (s *Server) buildLoopDetail(root queue.RootRef, e tickets.Epic, now time.Time) wire.EpicDetail {
	queued := s.queue.RootQueued(root)
	d := wire.EpicDetail{
		Address: root.String(), Kind: wire.EpicKindLoop, Summary: schema.Section(e.Body, "Summary"),
		State: epicStateFor(root, e, s.registry.StartedAtOf),
		Done:  e.DoneCount(), Total: e.TotalCount(), Rows: []wire.EpicDetailRow{},
		Queueable: queue.EpicQueueable(e, queued), Queued: queued,
		Draft: e.Status == string(schema.StatusDraft),
	}
	if epicMergeBlocked(e) {
		d.MergeBlockKind, d.MergeBlockReason, d.MergeBlockFiles = e.MergeBlockKind, e.MergeBlockReason, e.MergeBlockFiles
		d.Actions = mergeBlockActions(e.MergeBlockKind)
		d.ActionInfo = actionInfo(d.Actions)
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
func epicStats(e tickets.Epic, end, now time.Time) wire.EpicStats {
	var st wire.EpicStats
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

func epicFriction(f *schema.FrictionSummary) *wire.EpicFriction {
	if f == nil {
		return nil
	}
	out := &wire.EpicFriction{PausesByReason: f.PausesByReason, RunEvents: f.RunEvents, PausedSecs: f.TotalPausedSecs}
	out.WebShare = stats.WebShare(f.PauseEndings)
	return out
}

// baseRow is the part of a row both epic kinds share; status is e.RenderedStatus(t).
func baseRow(root queue.RootRef, e tickets.Epic, t tickets.Ticket, status tickets.RenderedStatus) wire.EpicDetailRow {
	return wire.EpicDetailRow{
		Address: root.TicketAddress(t.DisplayNumber()), Title: t.Title, RenderedStatus: status.Word(),
		Type: t.Type, Blockers: e.UnresolvedBlockers(t), ClosedAt: t.FinishedAt(), HadCodex: t.HadCodex,
	}
}

// rowFor builds a loop epic's full row for t.
func (s *Server) rowFor(root queue.RootRef, e tickets.Epic, t tickets.Ticket) wire.EpicDetailRow {
	status := e.RenderedStatus(t)
	row := baseRow(root, e, t, status)
	row.Elapsed, row.Cost = t.ElapsedTime, t.ActualCost
	if t.LongTicketTurns > 0 {
		row.LongTicketTurns, row.LongTicketThreshold = t.LongTicketTurns, s.settings.Load().LongTicketTurns
	}
	row.HarnessLabel = s.harness.LabelFor(tickets.Address{Project: root.Project, Epic: root.Epic, ID: t.DisplayNumber()}, t.Agent, row.RenderedStatus)
	if t.Parent != nil {
		row.Parent = root.TicketAddress(*t.Parent)
	}
	if at, ok := s.registry.StartedAtOf(row.Address); ok && status == tickets.StatusClaimed {
		row.ClaimedAt = at
	}
	evs := queue.ReadLogLenient(t)
	if open := tickets.OpenQuestion(evs); open != nil && status == tickets.StatusPausedAnswer {
		row.Asking, row.Question = true, open.Summary
	}
	if status == tickets.StatusPausedRepair {
		row.State = sessions.RepairState(evs, t.PausedClass())
		row.FixPrompt = sessions.FixAgentPrompt(s.projects.LaunchHarness(root.Project), row.Address, t)
	}
	if t.PausedClass() != "" {
		row.Reason = t.PauseReason.Label()
	}
	return row
}
