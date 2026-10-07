package server

import (
	"fmt"
	"sync"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// EventExplainVerdictChange streams when a queued ticket's explain verdict
// moves to a different word. Address is the ticket; clients re-explain it.
const EventExplainVerdictChange = "explain-verdict-change"

// Verdicts only the server's queue and slot state can give. The rest are the
// scheduler's own decision words (blocked, stalled, done, error, claimed, ...).
const (
	VerdictEligible       = "eligible"
	VerdictWaitingInQueue = "waiting in queue"
	VerdictConcurrencyCap = "concurrency cap"
	VerdictNotQueued      = "not queued"
)

// PendingRow is one queue entry with its explain verdict.
type PendingRow struct {
	Address string `json:"address"`
	Agent   string `json:"agent"`
	Verdict string `json:"verdict"`
	Reason  string `json:"reason,omitempty"`
}

// verdictLog remembers the verdict word last seen per queued ticket, to tell
// when one changed.
type verdictLog struct {
	mu   sync.Mutex
	seen map[string]string
}

// verdictOf is the full explain for one stored ticket.
func (s *Server) verdictOf(e tickets.Epic, t tickets.Ticket, addr tickets.Address) Explanation {
	d := ralphloop.TicketVerdict(e, ralphloop.WholeEpicScope(), t, t.Status == "claimed", false)
	switch d.Decision {
	case "stalled":
		return Explanation{Address: addr.String(), Verdict: d.Decision, Reason: d.Status} // needs-answer, needs-repair or draft
	case "unclaimed":
		if path, missing := s.unavailablePath(addr.Project); missing {
			return Explanation{Address: addr.String(), Verdict: VerdictProjectUnavailable, Reason: path + " does not exist"}
		}
		return s.scheduleVerdict(e, t, addr)
	}
	return Explanation{Address: addr.String(), Verdict: d.Decision, Reason: d.Reason}
}

// scheduleVerdict explains an unclaimed ticket that nothing in the store blocks:
// where it stands in the queue and the slots.
func (s *Server) scheduleVerdict(e tickets.Epic, t tickets.Ticket, addr tickets.Address) Explanation {
	ex := Explanation{Address: addr.String()}
	if s.refused.has(ex.Address) {
		ex.Verdict = VerdictClaimRereadMismatch
		return ex
	}
	root := addr.Project + ":" + addr.Epic
	ahead, queued := s.rootsAhead(root)
	switch {
	case !queued:
		ex.Verdict = VerdictNotQueued
	case s.registry.countRoot(root) >= s.perRootLimit():
		ex.Verdict, ex.Reason = VerdictConcurrencyCap, fmt.Sprintf("%d of %d agents in use for this epic", s.registry.countRoot(root), s.perRootLimit())
	case frontierIndex(e, t) >= s.perRootLimit()-s.registry.countRoot(root):
		ex.Verdict, ex.Reason = VerdictWaitingInQueue, "earlier tickets of the epic go first"
	default:
		limit, running := s.concurrencyLimit(), s.registry.count()
		free := limit - running
		switch {
		case free <= 0:
			ex.Verdict, ex.Reason = VerdictConcurrencyCap, fmt.Sprintf("%d of %d slots in use", running, limit)
		case ahead >= free:
			ex.Verdict, ex.Reason = VerdictWaitingInQueue, fmt.Sprintf("%d queued root(s) ahead for %d free slot(s)", ahead, free)
		default:
			ex.Verdict = VerdictEligible
		}
	}
	return ex
}

// frontierIndex is t's place among the epic's claimable tickets, or -1.
func frontierIndex(e tickets.Epic, t tickets.Ticket) int {
	for i, f := range ralphloop.Frontier(e) {
		if f.Identifier == t.Identifier {
			return i
		}
	}
	return -1
}

// rootsAhead counts the distinct idle roots queued before root, and whether
// root is queued at all. Running roots hold a slot already, so they don't count.
func (s *Server) rootsAhead(root string) (ahead int, queued bool) {
	seen := map[string]bool{}
	for _, item := range s.queued.list() {
		a, err := tickets.ParseAddress(item.Address, tickets.AddressContext{})
		if err != nil {
			continue
		}
		r := a.Project + ":" + a.Epic
		if r == root {
			return ahead, true
		}
		if !seen[r] && s.registry.countRoot(r) == 0 {
			seen[r] = true
			ahead++
		}
	}
	return ahead, false
}

// pendingRows explains every queue entry, in queue order.
func (s *Server) pendingRows() []PendingRow {
	rows := []PendingRow{}
	for _, item := range s.queued.list() {
		addr, err := tickets.ParseAddress(item.Address, tickets.AddressContext{})
		if err != nil {
			continue
		}
		ex, err := s.explainTicket(addr)
		if err != nil {
			ex = Explanation{Verdict: "error", Reason: err.Error()}
		}
		rows = append(rows, PendingRow{Address: item.Address, Agent: item.Agent, Verdict: ex.Verdict, Reason: ex.Reason})
	}
	return rows
}

// publishVerdictChanges streams one event per queued ticket whose verdict word
// differs from the last pass. A ticket seen for the first time only sets the
// baseline: the snapshot (and queue-changed) already told clients about it.
func (s *Server) publishVerdictChanges() {
	rows := s.pendingRows()
	l := &s.verdicts
	l.mu.Lock()
	defer l.mu.Unlock()
	next := make(map[string]string, len(rows))
	for _, r := range rows {
		next[r.Address] = r.Verdict
		if prev, ok := l.seen[r.Address]; ok && prev != r.Verdict {
			s.events.publish(EventExplainVerdictChange, r.Address)
		}
	}
	l.seen = next
}
