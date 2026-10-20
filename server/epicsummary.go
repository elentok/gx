package server

import (
	"slices"
	"time"

	"github.com/elentok/gx/tickets"
)

// EpicState is EpicSummary.State.
type EpicState string

const (
	EpicStateAwaitingMerge EpicState = "awaiting-merge"
	EpicStateRunning       EpicState = "running"
	EpicStateWaiting       EpicState = "waiting"
	EpicStateParked        EpicState = "parked"
	EpicStateDeadlocked    EpicState = "deadlocked"
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
	// Reason names what holds a Deadlocked epic.
	Reason string `json:"reason,omitempty"`
}

// epicSummaries summarizes every epic with a queue item, in queue order. A
// Merged epic and a map epic have no card.
func (s *Server) epicSummaries(all []TicketInfo) []EpicSummary {
	out := []EpicSummary{}
	seen := map[rootRef]bool{}
	recoveryOf := func(addr string) string { return recoveryState(s.parkHold.state(addr), addr, all) }
	for _, item := range s.queued.list() {
		addr, err := tickets.ParseAddress(item.Address, tickets.AddressContext{})
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
		out = append(out, summarizeEpic(root, epics[i], s.registry.startedAtOf, recoveryOf))
	}
	return out
}

// summarizeEpic applies the state rules, first match wins. A claimed ticket
// counts as runnable, but only one with a server claim time counts as running.
func summarizeEpic(root rootRef, e tickets.Epic, claimedAt func(string) (time.Time, bool), recoveryOf func(string) string) EpicSummary {
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
			sum.Parked = append(sum.Parked, info)
		case status == tickets.StatusClaimed:
			if at, ok := claimedAt(addr); ok {
				info.ClaimedAt = at
				sum.Active = append(sum.Active, info)
			}
		}
	}
	switch epicStateOf(e) {
	case epicDone:
		sum.State = EpicStateAwaitingMerge
	case epicRunnable:
		sum.State = EpicStateWaiting
		if len(sum.Active) > 0 {
			sum.State = EpicStateRunning
		}
	case epicParked:
		sum.State = EpicStateParked
	case epicCycle:
		sum.State = EpicStateDeadlocked
		sum.Reason = cycleReason(root, e)
	}
	if sum.State != EpicStateRunning {
		sum.Active = []SummaryTicket{}
	}
	return sum
}
