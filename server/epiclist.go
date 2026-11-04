package server

import (
	"cmp"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// followUpsEpicName is every project's contributor outbox epic.
const followUpsEpicName = "follow-ups"

// epicListed reports whether the Epics page can show e: one-offs, follow-ups
// and archived epics are never listed.
func epicListed(e tickets.Epic) bool {
	return e.Name != followUpsEpicName && !strings.HasSuffix(e.Name, "-archived") && !queue.IsOneOffEpic(e)
}

// epicListState is the one rule for a listed epic's state. A map has no
// queue state, and a loop epic is draft, finished or whatever the queue and
// its tickets say.
func epicListState(e tickets.Epic, queued, queueable bool, loopState wire.EpicState) wire.EpicListState {
	switch {
	case e.IsMap:
		return mapListState(e)
	case !e.MergedAt.IsZero():
		return wire.EpicListFinished
	case e.Status == string(schema.StatusDraft):
		return wire.EpicListDraft
	case loopState == wire.EpicStatePaused && epicMergeBlocked(e):
		return wire.EpicListMergeBlocked
	case loopState != wire.EpicStateWaiting && loopState != wire.EpicStateFinished:
		return wire.EpicListState(loopState)
	case queued:
		return wire.EpicListQueued
	case queueable:
		return wire.EpicListReady
	}
	return wire.EpicListNoOpenTickets
}

// mapListState: a map with a completed_at is finished. Otherwise it is
// designing once a ticket is claimed or done, and waiting to design before.
func mapListState(e tickets.Epic) wire.EpicListState {
	switch {
	case !e.CompletedAt.IsZero():
		return wire.EpicListFinished
	case slices.ContainsFunc(e.Tickets, func(t tickets.Ticket) bool {
		return t.Status == string(schema.StatusClaimed) || t.Status == string(schema.StatusDone)
	}):
		return wire.EpicListDesigning
	}
	return wire.EpicListWaitingToDesign
}

func (s *Server) epicList(w http.ResponseWriter, _ *http.Request) {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	list := wire.EpicList{Epics: []wire.EpicListRow{}, FollowUps: []wire.FollowUpCount{}}
	designs := s.designActivities()
	qidx := s.newQueueIndex()
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		list.Projects = append(list.Projects, project)
		epics, err := s.projects.Epics(project)
		if err != nil {
			continue
		}
		for _, e := range epics {
			if e.Name == followUpsEpicName {
				if open := openFollowUps(e); open > 0 {
					list.FollowUps = append(list.FollowUps, wire.FollowUpCount{Project: project, Open: open})
				}
			}
		}
		list.Epics = append(list.Epics, s.projectCards(project, epics, designs, qidx)...)
	}
	list.RecentlyFinished = s.settings.Load().RecentlyFinished
	list.Epics = capFinishedOneOffs(list.Epics, list.RecentlyFinished)
	sortCards(list.Epics, func(r wire.EpicListRow) int { return r.QueueSlot })
	writeJSON(w, list)
}

// projectCards builds one project's cards: follow-ups, one-offs, loop and map
// epics, then pending Design cards. A listed map's card shows its -impl epic,
// so that epic gets no row of its own.
func (s *Server) projectCards(project string, epics []tickets.Epic, designs map[string]designActivity, qidx queueIndex) []wire.EpicListRow {
	merged := map[string]bool{}
	byName := map[string]tickets.Epic{}
	for _, e := range epics {
		merged[e.Name] = !e.MergedAt.IsZero()
		byName[e.Name] = e
	}
	// joined maps a listed map's name to its -impl epic.
	joined := map[string]tickets.Epic{}
	for _, e := range epics {
		if impl, ok := byName[e.Name+tickets.ImplEpicSuffix]; ok && e.IsMap && epicListed(e) {
			joined[e.Name] = impl
		}
	}
	isJoinedImpl := func(e tickets.Epic) bool {
		name, ok := strings.CutSuffix(e.Name, tickets.ImplEpicSuffix)
		_, has := joined[name]
		return ok && has
	}

	var rows []wire.EpicListRow
	if fu, ok := byName[followUpsEpicName]; ok {
		if row, ok := followUpsCard(project, fu); ok {
			rows = append(rows, row)
		}
	}
	for _, e := range epics {
		rows = append(rows, s.oneOffCards(project, e, qidx)...)
	}
	for _, e := range epics {
		if !epicListed(e) || isJoinedImpl(e) {
			continue
		}
		row := s.epicListRow(project, e, qidx)
		row.BlockedByEpics = unmergedBlockers(project, e, merged)
		if e.IsMap {
			row.ProgressStep = mapProgress(e)
			var impl *wire.EpicListRow
			if implEpic, ok := joined[e.Name]; ok {
				r := s.epicListRow(project, implEpic, qidx)
				impl = &r
			}
			decorateDesignCard(&row, e, impl, designs[row.Address])
		}
		rows = append(rows, row)
	}
	return rows
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
			out = append(out, queue.RootRef{Project: project, Epic: name}.String())
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

func (s *Server) epicListRow(project string, e tickets.Epic, qidx queueIndex) wire.EpicListRow {
	root := queue.RootRef{Project: project, Epic: e.Name}
	queued := s.queue.RootQueued(root)
	queueable := queue.EpicQueueable(e, queued)
	row := wire.EpicListRow{
		Address: root.String(), Project: project, Name: e.Name, Kind: wire.EpicKindLoop,
		Done: e.DoneCount(), Total: e.TotalCount(),
		CompletedAt: e.CompletedAt, MergedAt: e.MergedAt, CreatedAt: e.CreatedAt,
		QueuedAt: cmp.Or(e.QueuedAt, e.StartedAt, e.MergedAt),
		Queued:   queued, Queueable: queueable,
		LastActivityAt: lastActivity(e),
	}
	if e.IsMap {
		row.Kind = wire.EpicKindMap
	}
	row.State = epicListState(e, queued, queueable, epicStateFor(root, e, s.registry.StartedAtOf))
	if row.State == wire.EpicListMergeBlocked {
		row.MergeBlockKind, row.MergeBlockReason = e.MergeBlockKind, e.MergeBlockReason
	}
	if !e.IsMap {
		row.QueueSlot = qidx.slot(root.String())
		decorateLoopCard(&row, row.QueueSlot)
		if n, ok := s.loopNeedsYou(root, e, row.State); ok {
			row.Section, row.StatePill, row.Reason, row.WaitingSince = wire.SectionNeedsYou, n.Pill, n.Reason, n.Since
			row.Actions = append([]wire.CardAction{n.Action}, n.More...)
		}
		setCardTime(&row, cardTimeSources{startedAt: e.StartedAt, enqueuedAt: qidx.enqueuedAt(root.String())})
	}
	return row
}
