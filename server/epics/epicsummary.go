package epics

import (
	"slices"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketlog"
)

// EpicSummaries summarizes every epic with a queue item, in queue order. A
// Merged epic and a map epic have no card.
func (s *Epics) EpicSummaries(all []wire.TicketInfo) []wire.EpicSummary {
	out := []wire.EpicSummary{}
	seen := map[queue.RootRef]bool{}
	recoveryOf := func(addr string) string { return ticketwrite.RecoveryState(s.Pauses.State(addr), addr, all) }
	for _, item := range s.Queue.List() {
		addr, err := tickets.ParseQueuedAddress(item.Address)
		if err != nil {
			continue
		}
		root := queue.RootOf(addr)
		if seen[root] {
			continue
		}
		seen[root] = true
		projEpics, err := s.Projects.Epics(root.Project)
		if err != nil {
			continue
		}
		i := slices.IndexFunc(projEpics, func(e tickets.Epic) bool { return e.Name == root.Epic })
		if i < 0 || projEpics[i].IsMap || !projEpics[i].MergedAt.IsZero() {
			continue
		}
		sum := summarizeEpic(root, projEpics[i], s.Runs.StartedAtOf, recoveryOf, queue.ReadLogLenient)
		for j := range sum.Active {
			sum.Active[j].NeedsApproval = s.needsApproval(sum.Active[j].Address)
			sum.Active[j].LaunchStep = s.launchStep(sum.Active[j].Address)
		}
		sum.Verdict, sum.VerdictReason = s.capLine(root, projEpics[i], sum.State == wire.EpicStateWaiting || sum.State == wire.EpicStateRunning)
		out = append(out, sum)
	}
	return out
}

// launchStep is the wire form of the step the run at address is launching in.
func (s *Epics) launchStep(address string) wire.LaunchStep {
	label, position := s.Runs.LaunchStepOf(address)
	return wire.LaunchStep{Label: label, Position: position}
}

// capLine is the verdict word and reason of the epic's next ready ticket while
// a cap holds it back; empty when active is false or no cap holds it.
func (s *Epics) capLine(root queue.RootRef, e tickets.Epic, active bool) (verdict, reason string) {
	if !active || s.CapHeld == nil {
		return "", ""
	}
	if ex, held := s.CapHeld(root, e); held {
		return ex.Verdict, ex.Reason
	}
	return "", ""
}

// needsApproval reports whether the live agent of address waits on a
// permission prompt. It reads the runner's status, so it runs only when a
// snapshot or a detail is built.
func (s *Epics) needsApproval(address string) bool {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return false
	}
	card, err := s.PermissionCard(addr)
	return err == nil && card != nil
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
func summarizeEpic(root queue.RootRef, e tickets.Epic, claimedAt func(string) (time.Time, bool), recoveryOf func(string) string, logOf func(tickets.Ticket) []tickets.LogEvent) wire.EpicSummary {
	sum := wire.EpicSummary{
		Address: root.String(), Name: e.Name, Project: root.Project, Done: e.DoneCount(), Total: e.TotalCount(),
		Active: []wire.SummaryTicket{}, Paused: []wire.SummaryTicket{},
	}
	for _, t := range e.Tickets {
		status := e.RenderedStatus(t)
		addr := tickets.Address{Project: root.Project, Epic: root.Epic, ID: t.DisplayNumber()}.String()
		info := wire.SummaryTicket{Address: addr, Title: t.Title, Status: status.Word()}
		switch {
		case IsPaused(e, t):
			info.Recovery, info.Reason = recoveryOf(addr), ticketlog.PauseDetail(e, t)
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
	oneOffs := root.Epic == queue.OneOffsEpic
	if oneOffs {
		sum.Done, sum.Total = 0, 0
	}
	sum.State = EpicStateFor(root, e, claimedAt)
	if sum.State == wire.EpicStatePaused {
		sum.Reason, sum.More = pausedEpicReason(e)
		switch {
		case root.Mergeable() && EpicMergeBlocked(e):
			sum.Reason, sum.Detail = wire.EpicReasonMergeBlocked, e.MergeBlockReason
			sum.MergeBlocked, sum.MergeBlockKind = true, e.MergeBlockKind
			sum.Actions = mergeBlockActions(e.MergeBlockKind)
		case StateOf(e) == StateCycle:
			sum.Reason, sum.Detail = wire.EpicReasonDeadlock, CycleReason(root, e)
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

// pausedEpicReason is the reason of the first paused ticket in e's ticket
// order and the count of the others. A draft is not paused, so a drafts-only
// epic has neither.
func pausedEpicReason(e tickets.Epic) (reason string, more int) {
	n := 0
	for _, t := range e.Tickets {
		if !IsPaused(e, t) || e.RenderedStatus(t) == tickets.StatusDraft {
			continue
		}
		if n == 0 {
			reason = ticketlog.PauseDetail(e, t)
		}
		n++
	}
	return reason, max(n-1, 0)
}

// EpicMergeBlocked reports whether e is done and unmerged, with its last merge blocked.
func EpicMergeBlocked(e tickets.Epic) bool {
	return StateOf(e) == StateDone && e.MergedAt.IsZero() && e.MergeBlockReason != ""
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
			if t.Type != string(schema.TypeMerge) || IsPaused(e, t) {
				return false
			}
			merging = true
		default:
			return false
		}
	}
	return merging
}

// EpicStateFor is the one place that decides an epic's state. Running needs a
// claimed, non-paused ticket that has a server claim time.
func EpicStateFor(root queue.RootRef, e tickets.Epic, claimedAt func(string) (time.Time, bool)) wire.EpicState {
	switch StateOf(e) {
	case StateDone:
		if !root.Mergeable() {
			return wire.EpicStateWaiting
		}
		if e.MergedAt.IsZero() {
			if EpicMergeBlocked(e) {
				return wire.EpicStatePaused
			}
			return wire.EpicStateAwaitingMerge
		}
		return wire.EpicStateFinished
	case StateRunnable:
		if root.Mergeable() && onlyMergeTicketsRemain(e) {
			return wire.EpicStateMerging
		}
		for _, t := range e.Tickets {
			if IsPaused(e, t) || e.RenderedStatus(t) != tickets.StatusClaimed {
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
