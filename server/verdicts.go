package server

import (
	"cmp"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/config"
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

	VerdictRateLimited = "rate-limited"

	VerdictWaitingOnAnswer    = "waiting on answer"
	VerdictWaitingReleased    = "waiting (slot released)"
	VerdictWaitingReleaseCapd = "waiting (slot held, release cap reached)"
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
	// A launch is tracked before the index may have reread its claim.
	claimed := t.Status == "claimed" || s.registry.has(addr.String())
	// A rate-limit hold outranks every other wait reason.
	if t, ok := s.registry.tracked(addr.String()); ok && t.RateLimited {
		return Explanation{Address: addr.String(), Verdict: rateLimitVerdict(t.RateLimitReset)}
	}
	// A live agent waiting for its answer outranks the stored status.
	if v, ok := s.waitingVerdict(addr); ok {
		return Explanation{Address: addr.String(), Verdict: v}
	}
	d := ralphloop.TicketVerdict(e, ralphloop.WholeEpicScope(), t, claimed, false)
	switch d.Decision {
	case ralphloop.DecisionStalled:
		return Explanation{Address: addr.String(), Verdict: string(d.Decision), Reason: d.Status} // asking, stuck or draft
	case ralphloop.DecisionUnclaimed:
		if path, missing := s.unavailablePath(addr.Project); missing {
			return Explanation{Address: addr.String(), Verdict: VerdictProjectUnavailable, Reason: path + " does not exist"}
		}
		return s.scheduleVerdict(e, t, addr)
	case ralphloop.DecisionBlocked:
		// The scheduler's reason lists only Blocked by: tokens, so a fork child
		// held by its parent alone would explain as blocked by nothing.
		if !e.ParentDone(t) {
			parent := "parent " + string(*t.Parent)
			if d.Reason == "" {
				d.Reason = parent
			} else {
				d.Reason += ", " + parent
			}
		}
	}
	return Explanation{Address: addr.String(), Verdict: string(d.Decision), Reason: d.Reason}
}

// rateLimitVerdict is the whole phrase a held run reads as: the verdict line
// joins verdict and reason with a colon, so the phrase is carried as the verdict.
func rateLimitVerdict(resetAt time.Time) string {
	if resetAt.IsZero() {
		return VerdictRateLimited
	}
	return VerdictRateLimited + ": resets " + resetAt.UTC().Format(time.RFC3339)
}

// waitingVerdict is the verdict of a ticket whose live agent waits for an answer.
func (s *Server) waitingVerdict(addr tickets.Address) (string, bool) {
	st, def := s.settings.Load(), config.DefaultExecutionQueueConfig()
	grace := cmp.Or(st.WaitingSlotGrace, def.WaitingSlotGrace)
	w := s.registry.waitStateOf(addr.String(), time.Now(), grace, st.MaxReleasedAgents)
	switch {
	case !w.Waiting:
		return "", false
	case w.Released:
		return VerdictWaitingReleased, true
	case w.CapHeld:
		return VerdictWaitingReleaseCapd, true
	}
	return VerdictWaitingOnAnswer, true
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
	if reason, missing := s.trunkMissing.get(addr.Project); missing {
		ex.Verdict, ex.Reason = VerdictTrunkMissing, reason
		return ex
	}
	if d, held := s.heldDiskOf(addr.Project); held {
		ex.Verdict, ex.Reason = VerdictDiskLow, diskLowReason(d)
		return ex
	}
	if h, held := s.claimHold(addr.Project, t, ticketAgent(t, s.rootAgent(root))); held {
		ex.Verdict, ex.Reason = h.Verdict, h.Reason
		return ex
	}
	// Every full cap is named, not just the first one hit.
	var full []string
	rootCap := s.perRootLimit(rootOf(addr))
	if n, limit := s.registry.countRoot(root), rootCap; n >= limit {
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
	case frontierIndex(e, t) >= rootCap-s.registry.countRoot(root):
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
		a, err := parseQueuedAddress(item.Address)
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
		// An unreadable project.json also fails the load; the Queue tab still
		// says why the project stopped.
		if reason, missing := s.trunkMissing.get(addr.Project); missing {
			return Explanation{Address: addr.String(), Verdict: VerdictTrunkMissing, Reason: reason}, nil
		}
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
	items := s.queued.list()
	// A ticket queued alone has its own row; its epic's entry adds none for it.
	covered := map[string]bool{}
	for _, item := range items {
		if addr, err := parseQueuedAddress(item.Address); err == nil && addr.ID != "" {
			covered[addr.String()] = true
		}
	}
	for _, item := range items {
		target, ok := s.queuedTarget(item, covered, epicsOf)
		if !ok {
			continue
		}
		ex, err := s.explainQueued(target, epicsOf)
		if err != nil {
			ex = Explanation{Verdict: "error", Reason: err.Error()}
		}
		rows = append(rows, PendingRow{Address: target.String(), Agent: item.Agent, Verdict: ex.Verdict, Reason: ex.Reason})
	}
	// A running ticket is no queue target; a held run still gets a row so the
	// verdict plumbing shows its hold.
	for _, t := range s.registry.rateLimitedRuns() {
		if !covered[t.Address] {
			rows = append(rows, PendingRow{Address: t.Address, Agent: t.Agent, Verdict: rateLimitVerdict(t.RateLimitReset)})
		}
	}
	return rows
}

// queuedTarget is the ticket a queue entry gets a row for: the ticket itself,
// or the next ticket of a whole-epic entry. It is false for an unparseable
// entry and for an epic entry whose next ticket is unknown or already covered;
// a chosen epic ticket is marked covered.
func (s *Server) queuedTarget(item QueueItem, covered map[string]bool, epicsOf func(project string) ([]tickets.Epic, error)) (tickets.Address, bool) {
	addr, err := parseQueuedAddress(item.Address)
	if err != nil {
		return tickets.Address{}, false
	}
	if addr.ID != "" {
		return addr, true
	}
	next, ok := s.nextQueuedTicket(addr, epicsOf)
	if !ok || covered[next.String()] {
		return tickets.Address{}, false
	}
	covered[next.String()] = true
	return next, true
}

// nextQueuedTicket is the ticket a whole-epic queue entry would start next, if
// its epic is readable and has a frontier.
func (s *Server) nextQueuedTicket(epic tickets.Address, epicsOf func(project string) ([]tickets.Epic, error)) (tickets.Address, bool) {
	epics, err := epicsOf(epic.Project)
	if err != nil {
		return tickets.Address{}, false
	}
	for _, e := range epics {
		if filepath.Base(e.Path) != epic.Epic {
			continue
		}
		t, ok := s.nextUntracked(epic.Project, e)
		return tickets.Address{Project: epic.Project, Epic: epic.Epic, ID: t.Identifier}, ok
	}
	return tickets.Address{}, false
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
