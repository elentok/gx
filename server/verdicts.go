package server

import (
	"fmt"
	"strings"
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
	if !queued {
		ex.Verdict = VerdictNotQueued
		return ex
	}
	// Every full cap is named, not just the first one hit.
	var full []string
	if n, limit := s.registry.countRoot(root), s.perRootLimit(); n >= limit {
		full = append(full, fmt.Sprintf("epic cap %d/%d", n, limit))
	}
	if n, limit, atCap := s.projectAtCap(addr.Project); atCap {
		full = append(full, fmt.Sprintf("project cap %d/%d", n, limit))
	}
	limit, running := s.concurrencyLimit(), s.registry.count()
	free := limit - running
	if free <= 0 {
		full = append(full, fmt.Sprintf("global cap %d/%d", running, limit))
	}
	switch {
	case len(full) > 0:
		ex.Verdict, ex.Reason = VerdictConcurrencyCap, strings.Join(full, ", ")
	case frontierIndex(e, t) >= s.perRootLimit()-s.registry.countRoot(root):
		ex.Verdict, ex.Reason = VerdictWaitingInQueue, "earlier tickets of the epic go first"
	case ahead >= free:
		ex.Verdict, ex.Reason = VerdictWaitingInQueue, fmt.Sprintf("%d queued root(s) ahead for %d free slot(s)", ahead, free)
	default:
		ex.Verdict = VerdictEligible
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

// loadResult is one project's epics (or the error getting them) for a pass.
type loadResult struct {
	epics []tickets.Epic
	err   error
}

// explainQueued is explainTicket with the project lookup shared across a pass.
func (s *Server) explainQueued(addr tickets.Address, epicsOf func(project string) ([]tickets.Epic, error)) (Explanation, error) {
	epics, err := epicsOf(addr.Project)
	if err != nil {
		return Explanation{}, err
	}
	return s.explainIn(epics, addr)
}

// pendingRows explains every queue entry, in queue order.
func (s *Server) pendingRows() []PendingRow {
	rows := []PendingRow{}
	byProject := map[string]loadResult{}
	epicsOf := func(project string) ([]tickets.Epic, error) {
		if r, ok := byProject[project]; ok {
			return r.epics, r.err
		}
		epics, err := s.projectEpics(project)
		byProject[project] = loadResult{epics, err}
		return epics, err
	}
	for _, item := range s.queued.list() {
		addr, err := tickets.ParseAddress(item.Address, tickets.AddressContext{})
		if err != nil {
			continue
		}
		ex, err := s.explainQueued(addr, epicsOf)
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
