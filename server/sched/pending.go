package sched

import (
	"path/filepath"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// loadResult is one project's epics (or the error getting them) for a pass.
type loadResult struct {
	epics []tickets.Epic
	err   error
}

// explainQueued is explainTicket with the project lookup shared across a pass.
func (x *Explainer) explainQueued(addr tickets.Address, epicsOf func(project string) ([]tickets.Epic, error)) (wire.Explanation, error) {
	epics, err := epicsOf(addr.Project)
	if err != nil {
		// An unreadable project.json also fails the load; the Queue tab still
		// says why the project stopped.
		if reason, missing := x.Rec.TrunkMissing.Get(addr.Project); missing {
			return wire.Explanation{Address: addr.String(), Verdict: wire.VerdictTrunkMissing, Reason: reason}, nil
		}
		return wire.Explanation{}, err
	}
	return x.explainIn(epics, addr)
}

// Pending explains every queue entry, in queue order.
func (x *Explainer) Pending() []wire.PendingRow {
	rows := []wire.PendingRow{}
	byProject := map[string]loadResult{}
	epicsOf := func(project string) ([]tickets.Epic, error) {
		if r, ok := byProject[project]; ok {
			return r.epics, r.err
		}
		epics, err := x.Projects.Epics(project)
		byProject[project] = loadResult{epics, err}
		return epics, err
	}
	items := x.Queue.List()
	// A ticket queued alone has its own row; its epic's entry adds none for it.
	covered := map[string]bool{}
	for _, item := range items {
		if addr, err := tickets.ParseQueuedAddress(item.Address); err == nil && addr.ID != "" {
			covered[addr.String()] = true
		}
	}
	for _, item := range items {
		target, ok := x.queuedTarget(item, covered, epicsOf)
		if !ok {
			continue
		}
		ex, err := x.explainQueued(target, epicsOf)
		if err != nil {
			ex = wire.Explanation{Verdict: "error", Reason: err.Error()}
		}
		rows = append(rows, wire.PendingRow{Address: target.String(), Agent: item.Agent, Verdict: ex.Verdict, Reason: ex.Reason})
	}
	// A running ticket is no queue target; a held run still gets a row so the
	// verdict plumbing shows its hold.
	for _, t := range x.Registry.RateLimitedRuns() {
		if !covered[t.Address] {
			rows = append(rows, wire.PendingRow{Address: t.Address, Agent: t.Agent, Verdict: RateLimitVerdict(t.RateLimitReset)})
		}
	}
	return rows
}

// queuedTarget is the ticket a queue entry gets a row for: the ticket itself,
// or the next ticket of a whole-epic entry. It is false for an unparseable
// entry and for an epic entry whose next ticket is unknown or already covered;
// a chosen epic ticket is marked covered.
func (x *Explainer) queuedTarget(item wire.QueueItem, covered map[string]bool, epicsOf func(project string) ([]tickets.Epic, error)) (tickets.Address, bool) {
	addr, err := tickets.ParseQueuedAddress(item.Address)
	if err != nil {
		return tickets.Address{}, false
	}
	if addr.ID != "" {
		return addr, true
	}
	t, ok := x.NextQueuedTicket(addr, epicsOf)
	next := tickets.Address{Project: addr.Project, Epic: addr.Epic, ID: t.Identifier}
	if !ok || covered[next.String()] {
		return tickets.Address{}, false
	}
	covered[next.String()] = true
	return next, true
}

// NextQueuedTicket is the ticket a whole-epic queue entry would start next, if
// its epic is readable and has a frontier.
func (x *Explainer) NextQueuedTicket(epic tickets.Address, epicsOf func(project string) ([]tickets.Epic, error)) (tickets.Ticket, bool) {
	epics, err := epicsOf(epic.Project)
	if err != nil {
		return tickets.Ticket{}, false
	}
	for _, e := range epics {
		if filepath.Base(e.Path) == epic.Epic {
			return x.Scheduler.NextUntracked(epic.Project, e)
		}
	}
	return tickets.Ticket{}, false
}

// PublishChanges streams one event per queued ticket whose verdict word
// differs from the last pass. A ticket seen for the first time only sets the
// baseline: the snapshot (and queue-changed) already told clients about it.
func (x *Explainer) PublishChanges() {
	rows := x.Pending()
	l := &x.verdicts
	l.mu.Lock()
	defer l.mu.Unlock()
	next := make(map[string]string, len(rows))
	for _, r := range rows {
		next[r.Address] = r.Verdict
		if prev, ok := l.seen[r.Address]; ok && prev != r.Verdict {
			x.Events.Publish(wire.EventExplainVerdictChange, r.Address)
		}
	}
	l.seen = next
}
