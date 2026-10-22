package server

import (
	"slices"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// EpicState is EpicSummary.State.
type EpicState string

const (
	EpicStateAwaitingMerge EpicState = "awaiting-merge"
	EpicStateFinished      EpicState = "finished"
	EpicStateRunning       EpicState = "running"
	// EpicStateMerging is an epic whose only unfinished tickets are merge tickets
	// that are open or claimed.
	EpicStateMerging EpicState = "merging"
	EpicStateWaiting EpicState = "waiting"
	// EpicStatePaused is an epic with nothing runnable: a paused ticket, a
	// dependency deadlock, or a blocked merge. EpicSummary.Reason says which.
	EpicStatePaused EpicState = "paused"
)

// The reasons a Paused epic gives for itself, when no ticket is paused.
const (
	EpicReasonDeadlock     = "Dependency deadlock"
	EpicReasonMergeBlocked = "Merge blocked"
)

// SummaryTicket is one ticket in an EpicSummary: an active one carries
// ClaimedAt, a Parked one carries Recovery and its unclipped Park reason.
type SummaryTicket struct {
	Address   string    `json:"address"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	ClaimedAt time.Time `json:"claimed_at,omitzero"`
	Recovery  string    `json:"recovery,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	// Summary is the open question's summary line, or the park reason without one.
	Summary string `json:"summary,omitempty"`
	// Answerable is set when a person's reply can go to the open question.
	Answerable bool `json:"answerable"`
	// Answered is set on an open ticket whose latest question has an answer.
	Answered bool `json:"answered"`
	// NeedsApproval is set on an active ticket whose agent waits on a permission prompt.
	NeedsApproval bool `json:"needs_approval"`
}

// EpicSummary is one epic holding a queue item, as the web page's cards show it.
type EpicSummary struct {
	Address string    `json:"address"`
	Name    string    `json:"name"`
	Project string    `json:"project"`
	State   EpicState `json:"state"`
	Done    int       `json:"done"`
	Total   int       `json:"total"`
	// Active is the running tickets, listed only while the epic is Running.
	Active []SummaryTicket `json:"active"`
	Parked []SummaryTicket `json:"parked"`
	// HasRepair is set when a parked ticket is broken (needs-repair or error), so
	// both clients show a fault in the alarm color instead of a question.
	HasRepair bool `json:"has_repair"`
	// Reason is why a Paused epic is paused: the reason of its first paused
	// ticket, else "Dependency deadlock" or "Merge blocked". It is empty for a
	// Paused epic that holds only drafts.
	Reason string `json:"reason,omitempty"`
	// More is the number of paused tickets beyond the first, whose reason Reason shows.
	More int `json:"more,omitempty"`
	// Detail is the cycle that deadlocks the epic, or why its merge is blocked.
	Detail string `json:"detail,omitempty"`
	// MergeBlocked is set on a Paused epic whose last merge failed, so clients
	// keep offering the merge action.
	MergeBlocked bool `json:"merge_blocked,omitempty"`
	// MergeBlockKind is the pause reason of a blocked merge; empty for a needs-rebase block.
	MergeBlockKind string `json:"merge_block_kind,omitempty"`
}

// epicSummaries summarizes every epic with a queue item, in queue order. A
// Merged epic and a map epic have no card.
func (s *Server) epicSummaries(all []TicketInfo) []EpicSummary {
	out := []EpicSummary{}
	seen := map[rootRef]bool{}
	recoveryOf := func(addr string) string { return recoveryState(s.parkHold.state(addr), addr, all) }
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
		epics, err := s.projectEpics(root.Project)
		if err != nil {
			continue
		}
		i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == root.Epic })
		if i < 0 || epics[i].IsMap || !epics[i].MergedAt.IsZero() {
			continue
		}
		sum := summarizeEpic(root, epics[i], s.registry.startedAtOf, recoveryOf, readLogLenient)
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
	return end != nil && end.Type == tickets.EventParkAnswer
}

// summarizeEpic applies the state rules, first match wins. A claimed ticket
// counts as runnable, but only one with a server claim time counts as running.
func summarizeEpic(root rootRef, e tickets.Epic, claimedAt func(string) (time.Time, bool), recoveryOf func(string) string, logOf func(tickets.Ticket) []tickets.LogEvent) EpicSummary {
	sum := EpicSummary{
		Address: root.String(), Name: e.Name, Project: root.Project, Done: e.DoneCount(), Total: e.TotalCount(),
		Active: []SummaryTicket{}, Parked: []SummaryTicket{},
	}
	for _, t := range e.Tickets {
		status := e.RenderedStatus(t)
		addr := tickets.Address{Project: root.Project, Epic: root.Epic, ID: t.DisplayNumber()}.String()
		info := SummaryTicket{Address: addr, Title: t.Title, Status: status.Word()}
		switch {
		case isParked(e, t):
			info.Recovery, info.Reason = recoveryOf(addr), tickets.ParkReason(e, t)
			open := tickets.OpenQuestion(logOf(t))
			info.Summary = info.Reason
			if open != nil {
				info.Summary = open.Summary
			}
			info.Answerable = status == tickets.StatusNeedsAnswer && open != nil
			sum.Parked = append(sum.Parked, info)
			sum.HasRepair = sum.HasRepair || status == tickets.StatusNeedsRepair || status == tickets.StatusError
		case status == tickets.StatusOpen && answeredLatest(logOf(t)):
			// An answered gone-agent ticket is open again until gx claims it, yet a
			// person still wants to see it was answered.
			info.Answered = true
			sum.Parked = append(sum.Parked, info)
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
	if sum.State == EpicStatePaused {
		sum.Reason, sum.More = pausedEpicReason(e)
		switch {
		case root.Epic != OneOffsEpic && epicMergeBlocked(e):
			sum.Reason, sum.Detail = EpicReasonMergeBlocked, e.LandBlockReason
			sum.MergeBlocked, sum.MergeBlockKind = true, e.LandBlockKind
		case epicStateOf(e) == epicCycle:
			sum.Reason, sum.Detail = EpicReasonDeadlock, cycleReason(root, e)
		}
	}
	if sum.State != EpicStateRunning && sum.State != EpicStateMerging {
		sum.Active = []SummaryTicket{}
	}
	return sum
}

// pausedEpicReason is the reason of the first paused ticket in e's ticket
// order and the count of the others. A draft is not paused, so a drafts-only
// epic has neither.
func pausedEpicReason(e tickets.Epic) (reason string, more int) {
	n := 0
	for _, t := range e.Tickets {
		if !isParked(e, t) || e.RenderedStatus(t) == tickets.StatusDraft {
			continue
		}
		if n == 0 {
			reason = tickets.ParkReason(e, t)
		}
		n++
	}
	return reason, max(n-1, 0)
}

// epicMergeBlocked reports whether e is done and unmerged, with its last merge blocked.
func epicMergeBlocked(e tickets.Epic) bool {
	return epicStateOf(e) == epicDone && e.MergedAt.IsZero() && e.LandBlockReason != ""
}

// onlyMergeTicketsRemain reports whether every ticket is done or cancelled
// except merge tickets that are open or claimed. A parked merge ticket does not
// count, so the epic falls through to Parked.
func onlyMergeTicketsRemain(e tickets.Epic) bool {
	merging := false
	for _, t := range e.Tickets {
		switch e.RenderedStatus(t) {
		case tickets.StatusDone, tickets.StatusCancelled:
		case tickets.StatusOpen, tickets.StatusClaimed:
			if t.Type != string(schema.TypeMerge) || isParked(e, t) {
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
// claimed, non-parked ticket that has a server claim time.
func epicStateFor(root rootRef, e tickets.Epic, claimedAt func(string) (time.Time, bool)) EpicState {
	switch epicStateOf(e) {
	case epicDone:
		if root.Epic == OneOffsEpic {
			return EpicStateWaiting
		}
		if e.MergedAt.IsZero() {
			if epicMergeBlocked(e) {
				return EpicStatePaused
			}
			return EpicStateAwaitingMerge
		}
		return EpicStateFinished
	case epicRunnable:
		if root.Epic != OneOffsEpic && onlyMergeTicketsRemain(e) {
			return EpicStateMerging
		}
		for _, t := range e.Tickets {
			if isParked(e, t) || e.RenderedStatus(t) != tickets.StatusClaimed {
				continue
			}
			addr := tickets.Address{Project: root.Project, Epic: root.Epic, ID: t.DisplayNumber()}.String()
			if _, ok := claimedAt(addr); ok {
				return EpicStateRunning
			}
		}
		return EpicStateWaiting
	default:
		return EpicStatePaused
	}
}
