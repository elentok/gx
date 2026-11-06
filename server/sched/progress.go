package sched

import (
	"context"
	"sort"
	"time"

	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/ralphloop/schedule"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
)

// DefaultProgressInterval is how often an epic with a running ticket gets a Progress message.
const DefaultProgressInterval = 15 * time.Minute

// ProgressTimerDeps are the parts the progress timer reads and sends through.
type ProgressTimerDeps struct {
	Registry *runs.Registry
	Projects *projects.Registry
	Writer   *ticketwrite.Writer
	Chat     *notify.ServerChat
	// Interval is the time between messages; zero means DefaultProgressInterval.
	Interval time.Duration
}

// ProgressTimer sends one Progress message per interval per epic while a
// ticket runs in it. It follows the run registry, so one timer per epic holds
// whatever runner or harness started the tickets. State is in memory only: a
// restart starts every epic's timer again.
type ProgressTimer struct {
	d    ProgressTimerDeps
	next map[string]time.Time // epic key -> next tick
}

// NewProgressTimer builds the timer; Run drives it.
func NewProgressTimer(d ProgressTimerDeps) *ProgressTimer {
	if d.Interval <= 0 {
		d.Interval = DefaultProgressInterval
	}
	return &ProgressTimer{d: d, next: map[string]time.Time{}}
}

// Run polls the registry until ctx ends. The poll is a fraction of the
// interval so a tick is never late by more than that fraction.
func (p *ProgressTimer) Run(ctx context.Context) {
	poll := min(max(p.d.Interval/10, 5*time.Millisecond), time.Second)
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			p.Tick(now)
		}
	}
}

// Tick runs one pass as of now; tests call it directly.
func (p *ProgressTimer) Tick(now time.Time) {
	byEpic := map[string][]runs.TrackedRun{}
	addrs := map[string]tickets.Address{}
	for _, run := range p.d.Registry.List() {
		if run.Watched {
			continue
		}
		addr, err := tickets.ParseAddress(run.Address, tickets.AddressContext{})
		if err != nil {
			continue
		}
		key := addr.Project + "/" + addr.Epic
		byEpic[key] = append(byEpic[key], run)
		addrs[key] = addr
	}
	for key := range p.next {
		if _, ok := byEpic[key]; !ok {
			delete(p.next, key)
		}
	}
	for key, rs := range byEpic {
		due, started := p.next[key]
		if !started {
			p.next[key] = now.Add(p.d.Interval)
			continue
		}
		if now.Before(due) {
			continue
		}
		p.next[key] = due.Add(p.d.Interval)
		p.send(addrs[key], rs, now)
	}
}

func (p *ProgressTimer) send(addr tickets.Address, rs []runs.TrackedRun, now time.Time) {
	epic, ok := p.d.Writer.FreshEpic(addr.Project, addr.Epic)
	if !ok {
		return
	}
	titles := map[string]string{}
	for _, t := range epic.Tickets {
		titles[t.Identifier] = t.Title
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].Address < rs[j].Address })
	facts := notify.ProgressFacts{Counts: schedule.CountsOf(epic)}
	for _, r := range rs {
		ra, err := tickets.ParseAddress(r.Address, tickets.AddressContext{})
		if err != nil {
			continue
		}
		var elapsed int
		if !r.StartedAt.IsZero() {
			elapsed = int(now.Sub(r.StartedAt).Seconds())
		}
		facts.Running = append(facts.Running, notify.ProgressRunning{Identifier: ra.ID, Title: titles[ra.ID], ElapsedSeconds: max(0, elapsed)})
	}
	p.d.Chat.Progress(addr.Project, p.d.Projects.ChatOverride(addr.Project), addr.Epic, facts)
}
