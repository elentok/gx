package server

import (
	"slices"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// epicSummaries summarizes every epic with a queue item, in queue order. A
// Merged epic and a map epic have no card.
func (s *Server) epicSummaries(all []wire.TicketInfo) []wire.EpicSummary {
	out := []wire.EpicSummary{}
	seen := map[rootRef]bool{}
	recoveryOf := func(addr string) string { return recoveryState(s.pauseHold.state(addr), addr, all) }
	for _, item := range s.queued.list() {
		addr, err := parseQueuedAddress(item.Address)
		if err != nil {
			continue
		}
		root := rootOf(addr)
		if seen[root] {
			continue
		}
		seen[root] = true
		epics, err := s.projects.Epics(root.Project)
		if err != nil {
			continue
		}
		i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == root.Epic })
		if i < 0 || epics[i].IsMap || !epics[i].MergedAt.IsZero() {
			continue
		}
		sum := summarizeEpic(root, epics[i], s.registry.StartedAtOf, recoveryOf, readLogLenient)
		for j := range sum.Active {
			sum.Active[j].NeedsApproval = s.needsApproval(sum.Active[j].Address)
		}
		out = append(out, sum)
	}
	return out
}

// needsApproval reports whether the live agent of address waits on a
// permission prompt. It reads the runner's status, so it runs only when a
// snapshot or a detail is built.
func (s *Server) needsApproval(address string) bool {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return false
	}
	card, err := s.permissionCard(addr)
	return err == nil && card != nil
}

// readLogLenient reads a ticket's log for the snapshot, where an unreadable log
// only costs the question details.
func readLogLenient(t tickets.Ticket) []tickets.LogEvent {
	evs, _ := tickets.ReadLog(t.Path)
	return evs
}

// answeredLatest reports whether the latest question has an answer.
func answeredLatest(evs []tickets.LogEvent) bool {
	rounds := tickets.Rounds(evs)
	if len(rounds) == 0 {
		return false
	}
	end := rounds[len(rounds)-1].End
	return end != nil && end.Type == tickets.EventPauseAnswer
}

// summarizeEpic applies the state rules, first match wins. A claimed ticket
// counts as runnable, but only one with a server claim time counts as running.
func summarizeEpic(root rootRef, e tickets.Epic, claimedAt func(string) (time.Time, bool), recoveryOf func(string) string, logOf func(tickets.Ticket) []tickets.LogEvent) wire.EpicSummary {
	sum := wire.EpicSummary{
		Address: root.String(), Name: e.Name, Project: root.Project, Done: e.DoneCount(), Total: e.TotalCount(),
		Active: []wire.SummaryTicket{}, Paused: []wire.SummaryTicket{},
	}
	for _, t := range e.Tickets {
		status := e.RenderedStatus(t)
		addr := tickets.Address{Project: root.Project, Epic: root.Epic, ID: t.DisplayNumber()}.String()
		info := wire.SummaryTicket{Address: addr, Title: t.Title, Status: status.Word()}
		switch {
		case isPaused(e, t):
			info.Recovery, info.Reason = recoveryOf(addr), tickets.PauseDetail(e, t)
			info.ReasonLabel = t.PauseReason.Label()
			open := tickets.OpenQuestion(logOf(t))
			info.Summary = info.Reason
			if open != nil {
				info.Summary = open.Summary
			}
			info.Answerable = status == tickets.StatusPausedAnswer && open != nil
			sum.Paused = append(sum.Paused, info)
			sum.HasRepair = sum.HasRepair || status == tickets.StatusPausedRepair || status == tickets.StatusError
		case status == tickets.StatusOpen && answeredLatest(logOf(t)):
			// An answered gone-agent ticket is open again until gx claims it, yet a
			// person still wants to see it was answered.
			info.Answered = true
			sum.Paused = append(sum.Paused, info)
		case status == tickets.StatusClaimed:
			if at, ok := claimedAt(addr); ok {
				info.ClaimedAt = at
				sum.Active = append(sum.Active, info)
			}
		}
	}
	// The one-offs root has no end to merge and no total to count towards.
	oneOffs := root.Epic == OneOffsEpic
	if oneOffs {
		sum.Done, sum.Total = 0, 0
	}
	sum.State = epicStateFor(root, e, claimedAt)
	if sum.State == wire.EpicStatePaused {
		sum.Reason, sum.More = pausedEpicReason(e)
		switch {
		case isMergeableEpic(root) && epicMergeBlocked(e):
			sum.Reason, sum.Detail = wire.EpicReasonMergeBlocked, e.MergeBlockReason
			sum.MergeBlocked, sum.MergeBlockKind = true, e.MergeBlockKind
			sum.Actions = mergeBlockActions(e.MergeBlockKind)
		case epicStateOf(e) == epicCycle:
			sum.Reason, sum.Detail = wire.EpicReasonDeadlock, cycleReason(root, e)
		}
	}
	if !showsActive(sum.State) {
		sum.Active = []wire.SummaryTicket{}
	}
	return sum
}

// showsActive reports whether an epic in state lists its claimed tickets.
func showsActive(state wire.EpicState) bool {
	return state == wire.EpicStateRunning || state == wire.EpicStateMerging
}

// isMergeableEpic reports whether root can ever merge; the one-offs inbox never does.
func isMergeableEpic(root rootRef) bool {
	return root.Epic != OneOffsEpic
}

// pausedEpicReason is the reason of the first paused ticket in e's ticket
// order and the count of the others. A draft is not paused, so a drafts-only
// epic has neither.
func pausedEpicReason(e tickets.Epic) (reason string, more int) {
	n := 0
	for _, t := range e.Tickets {
		if !isPaused(e, t) || e.RenderedStatus(t) == tickets.StatusDraft {
			continue
		}
		if n == 0 {
			reason = tickets.PauseDetail(e, t)
		}
		n++
	}
	return reason, max(n-1, 0)
}

// epicMergeBlocked reports whether e is done and unmerged, with its last merge blocked.
func epicMergeBlocked(e tickets.Epic) bool {
	return epicStateOf(e) == epicDone && e.MergedAt.IsZero() && e.MergeBlockReason != ""
}

// onlyMergeTicketsRemain reports whether every ticket is done or cancelled
// except merge tickets that are open or claimed. A paused merge ticket does not
// count, so the epic falls through to Paused.
func onlyMergeTicketsRemain(e tickets.Epic) bool {
	merging := false
	for _, t := range e.Tickets {
		switch e.RenderedStatus(t) {
		case tickets.StatusDone, tickets.StatusCancelled:
		case tickets.StatusOpen, tickets.StatusClaimed:
			if t.Type != string(schema.TypeMerge) || isPaused(e, t) {
				return false
			}
			merging = true
		default:
			return false
		}
	}
	return merging
}

// epicStateFor is the one place that decides an epic's state. Running needs a
// claimed, non-paused ticket that has a server claim time.
func epicStateFor(root rootRef, e tickets.Epic, claimedAt func(string) (time.Time, bool)) wire.EpicState {
	switch epicStateOf(e) {
	case epicDone:
		if !isMergeableEpic(root) {
			return wire.EpicStateWaiting
		}
		if e.MergedAt.IsZero() {
			if epicMergeBlocked(e) {
				return wire.EpicStatePaused
			}
			return wire.EpicStateAwaitingMerge
		}
		return wire.EpicStateFinished
	case epicRunnable:
		if isMergeableEpic(root) && onlyMergeTicketsRemain(e) {
			return wire.EpicStateMerging
		}
		for _, t := range e.Tickets {
			if isPaused(e, t) || e.RenderedStatus(t) != tickets.StatusClaimed {
				continue
			}
			addr := tickets.Address{Project: root.Project, Epic: root.Epic, ID: t.DisplayNumber()}.String()
			if _, ok := claimedAt(addr); ok {
				return wire.EpicStateRunning
			}
		}
		return wire.EpicStateWaiting
	default:
		return wire.EpicStatePaused
	}
}
