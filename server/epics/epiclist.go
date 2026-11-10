package epics

import (
	"cmp"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/server/epics/prstate"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/triage"
)

// followUpsEpicName is every project's contributor outbox epic.
const followUpsEpicName = "follow-ups"

// epicListed reports whether the Epics page can show e: one-offs, follow-ups
// and archived epics are never listed.
func epicListed(e tickets.Epic) bool {
	return e.Name != followUpsEpicName && !strings.HasSuffix(e.Name, "-archived") && !tickets.IsOneOffEpic(e)
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
	case loopState == wire.EpicStatePaused && EpicMergeBlocked(e):
		return wire.EpicListMergeBlocked
	case loopState == wire.EpicStatePaused && prstate.Blocked(e):
		return wire.EpicListState(e.MergeBlockKind)
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

// EpicList serves the Epics page.
func (s *Epics) EpicList(w http.ResponseWriter, _ *http.Request) {
	list, err := s.cardList()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, list)
}

// NeedsYouCounts counts the Needs-you cards per workspace and in total. A
// One-off card counts in the total only.
func (s *Epics) NeedsYouCounts() (perWorkspace map[string]int, total int) {
	perWorkspace = map[string]int{}
	list, err := s.cardList()
	if err != nil {
		return perWorkspace, 0
	}
	for _, r := range list.Epics {
		if r.Section != wire.SectionNeedsYou {
			continue
		}
		total++
		if r.Workspace != "" {
			perWorkspace[r.Workspace]++
		}
	}
	return perWorkspace, total
}

// applyWorkspaces names the workspace of each Design and epic card and moves a
// held card to On hold: same status words, no Needs-you pill, a Take off hold action.
func (s *Epics) applyWorkspaces(rows []wire.EpicListRow) {
	if s.Workspaces == nil {
		return
	}
	for i := range rows {
		r := &rows[i]
		if r.CardKind != wire.CardKindDesign && r.CardKind != wire.CardKindImplementation {
			continue
		}
		r.Workspace = s.Workspaces.Workspace(r.Address)
		if !s.Workspaces.IsHeld(r.Address) {
			continue
		}
		if r.Section == wire.SectionNeedsYou {
			r.Reason = ""
		}
		r.Section, r.StatePill, r.WaitingSince, r.CountdownEndsAt = wire.SectionOnHold, "", time.Time{}, time.Time{}
		r.Verdict, r.LaunchStep = "", wire.LaunchStep{}
		r.Actions = []wire.CardAction{{Kind: wire.CardActionUnhold, Label: "Take off hold", Target: r.Addresses[0]}}
		setCardTime(r, cardTimeSources{})
	}
}

// cardList builds the page's cards, sorted, with workspaces and holds applied.
func (s *Epics) cardList() (wire.EpicList, error) {
	dirs, err := tickets.ProjectDirs(s.TicketStore)
	if err != nil {
		return wire.EpicList{}, err
	}
	list := wire.EpicList{Epics: []wire.EpicListRow{}, FollowUps: []wire.FollowUpCount{}}
	designs := s.designActivities()
	triageLive := s.triageSessions()
	qidx := s.newQueueIndex()
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		list.Projects = append(list.Projects, project)
		epics, err := s.Projects.Epics(project)
		if err != nil {
			continue
		}
		rec, err := triage.NewStore(dir).LoadLive()
		if err != nil {
			s.Log.Warn("follow-ups card: read triage", "project", project, "err", err)
		}
		for _, e := range epics {
			if e.Name == followUpsEpicName {
				if c, ok := followUpCount(project, e, rec, triageLive[project]); ok {
					list.FollowUps = append(list.FollowUps, c)
				}
			}
		}
		list.Epics = append(list.Epics, s.projectCards(project, epics, designs, rec, triageLive, qidx)...)
	}
	list.RecentlyFinished = s.Settings.Load().RecentlyFinished
	list.Epics = capFinishedOneOffs(list.Epics, list.RecentlyFinished)
	s.applyWorkspaces(list.Epics)
	s.applyFollowUpWorkspaces(list.Epics, list.FollowUps)
	sortCards(list.Epics, func(r wire.EpicListRow) int { return r.QueueSlot })
	return list, nil
}

// projectCards builds one project's cards: follow-ups, one-offs, loop and map
// epics, then pending Design cards. A listed map's card shows its -impl epic,
// so that epic gets no row of its own.
func (s *Epics) projectCards(project string, epics []tickets.Epic, designs map[string]designActivity, rec *triage.Record, triageLive map[string]bool, qidx queueIndex) []wire.EpicListRow {
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
		if row, ok := followUpsCard(project, fu, rec, triageLive[project]); ok {
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
			row.ProgressStep = queue.MapProgress(e)
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

// applyFollowUpWorkspaces gives each Follow-ups card and count entry the
// workspaces of its project: those of its Design and Implementation cards, or the
// Default workspace when it has none. Call it after applyWorkspaces.
func (s *Epics) applyFollowUpWorkspaces(rows []wire.EpicListRow, counts []wire.FollowUpCount) {
	if s.Workspaces == nil {
		return
	}
	byProject := map[string][]string{}
	for _, r := range rows {
		if r.Workspace != "" && !slices.Contains(byProject[r.Project], r.Workspace) {
			byProject[r.Project] = append(byProject[r.Project], r.Workspace)
		}
	}
	of := func(project string) []string {
		if ws := byProject[project]; len(ws) > 0 {
			return ws
		}
		// A project name is never a member key, so this is the Default workspace.
		return []string{s.Workspaces.Workspace(project)}
	}
	for i := range rows {
		if rows[i].CardKind == wire.CardKindFollowUps {
			rows[i].Workspaces = of(rows[i].Project)
		}
	}
	for i := range counts {
		counts[i].Workspaces = of(counts[i].Project)
	}
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

func (s *Epics) epicListRow(project string, e tickets.Epic, qidx queueIndex) wire.EpicListRow {
	root := queue.RootRef{Project: project, Epic: e.Name}
	queued := s.Queue.RootQueued(root)
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
	row.State = epicListState(e, queued, queueable, EpicStateFor(root, e, s.Delivery(project), s.Runs.StartedAtOf))
	if row.State == wire.EpicListMergeBlocked {
		row.MergeBlockKind, row.MergeBlockReason = e.MergeBlockKind, e.MergeBlockReason
	}
	if !e.IsMap {
		row.QueueSlot = qidx.slot(root.String())
		decorateLoopCard(&row, row.QueueSlot)
		row.Verdict, row.Reason = s.capLine(root, e, row.Queued)
		for _, t := range e.Tickets {
			if e.RenderedStatus(t) != tickets.StatusClaimed {
				continue
			}
			if row.LaunchStep = s.launchStep(root.TicketAddress(t.DisplayNumber())); row.LaunchStep != (wire.LaunchStep{}) {
				break
			}
		}
		if n, ok := s.loopNeedsYou(root, e, row.State); ok {
			row.Verdict, row.LaunchStep = "", wire.LaunchStep{}
			row.Section, row.StatePill, row.Reason, row.WaitingSince = wire.SectionNeedsYou, n.Pill, n.Reason, n.Since
			row.Actions = append([]wire.CardAction{n.Action}, n.More...)
		}
		s.decoratePR(&row, root, e)
		setCardTime(&row, cardTimeSources{startedAt: e.StartedAt, enqueuedAt: qidx.enqueuedAt(root.String())})
	}
	return row
}
