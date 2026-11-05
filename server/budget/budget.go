// Package budget owns the daily spend budget: the ledger, the soft and hard
// limit latches, the override and increase, the poll that meters live
// iterations, the budget kill, the hard-limit grace countdown, the budget
// notices and budget-changed events.
package budget

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/metrics"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server/broker"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/tickets"
)

const defaultKillGrace = 15 * time.Second

// Config is the part of the server config the budget reads.
type Config struct {
	StateDir  string
	SoftLimit float64
	HardLimit float64
	// KillGrace is the wait between ctrl+c and closing a pane; zero means the default.
	KillGrace time.Duration
	// PollInterval is how often live iterations are metered; zero means limits.DefaultPoll.
	PollInterval time.Duration
	// HardLimitGrace is how long a reload-lowered hard limit waits before it
	// stops agents; zero means one minute.
	HardLimitGrace time.Duration
	// EventInterval is the throttle window of budget-changed; zero means DefaultEventInterval.
	EventInterval time.Duration
	Env           []string
	// ExtraUsageCheck reads the subscription state; nil means subscription.CheckFresh.
	ExtraUsageCheck func() subscription.State
}

// Host is what the budget needs from the root.
type Host interface {
	// PauseTicket pauses the ticket with a kind and reason (the root's one way to pause).
	PauseTicket(scratchDir string, addr tickets.Address, ticketPath string, kind events.Kind, reason string, run runs.Run) error
	// WriteFollowUp files the follow-up, or adds an occurrence line to the open draft with the same key.
	WriteFollowUp(from tickets.Address, f FollowUp) (string, error)
}

// Deps is what the budget needs from the root.
type Deps struct {
	Config   Config
	Settings *settings.Holder
	Projects *projects.Registry
	Index    *index.Index
	Registry *runs.Registry
	Harness  *harnesschoice.Harness
	Events   *broker.Broker
	Guards   *runs.Guards
	Chat     *ralphloop.ServerChat
	Log      *slog.Logger
	Host     Host
}

// Budget is the ledger and everything that acts on it.
type Budget struct {
	cfg      Config
	settings *settings.Holder
	projects *projects.Registry
	idx      *index.Index
	registry *runs.Registry
	harness  *harnesschoice.Harness
	events   *broker.Broker
	guards   *runs.Guards
	chat     *ralphloop.ServerChat
	log      *slog.Logger
	host     Host

	ledger *ledger
	notes  notes
	grace  hardGrace

	extraUsage extraUsageNotes
	evs        *eventFeed
	costOf     func(wire.IterationInfo) (float64, bool) // swapped in tests
}

// New loads the ledger from the state dir.
func New(d Deps) (*Budget, error) {
	l, err := openLedger(d.Config.StateDir, d.Config.SoftLimit, d.Config.HardLimit)
	if err != nil {
		return nil, err
	}
	return &Budget{
		cfg: d.Config, settings: d.Settings, projects: d.Projects, idx: d.Index, registry: d.Registry,
		harness: d.Harness, events: d.Events, guards: d.Guards, chat: d.Chat, log: d.Log, host: d.Host,
		ledger: l,
		evs:    newEventFeed(d.Config.EventInterval, func() { d.Events.Publish(wire.EventBudgetChanged, "") }),
		costOf: iterationCostIn(d.Config.Env),
	}, nil
}

// spent is the spend the limits count: today's total beyond the override.
func spent(st wire.BudgetStatus) float64 { return st.Total - st.Override }

// Status is the budget as of now.
func (b *Budget) Status(now time.Time) wire.BudgetStatus {
	latch := b.ledger.latches(now)
	soft, hard := b.ledger.limits()
	day := now.Format(dayLayout)
	return wire.BudgetStatus{
		Day:          day,
		Total:        b.ledger.today(now),
		Projects:     b.ledger.byProject(day),
		Roots:        b.ledger.byRoot(day),
		SoftLimit:    soft,
		HardLimit:    hard,
		BudgetPaused: latch.Soft,
		HardLatched:  latch.Hard,
		Override:     latch.Override,
	}
}

// DaysCopy is a copy of the per-day totals, safe to read without the lock.
func (b *Budget) DaysCopy() map[string]float64 { return b.ledger.daysCopy() }

// SoftReached holds new starts back; the hard limit implies it.
func (b *Budget) SoftReached(now time.Time) bool { return b.ledger.latches(now).Soft }

// HardStopAt is when the running hard-limit countdown ends; zero when no
// reload-lowered limit is being held.
func (b *Budget) HardStopAt() time.Time {
	if b.ledger.held() {
		return b.grace.deadline()
	}
	return time.Time{}
}

// override lifts the latches so work may resume. The ledger is saved
// before it returns, so a restart cannot bring the latch back.
func (b *Budget) override(now time.Time) (wire.BudgetResult, error) {
	if !b.ledger.latches(now).Soft {
		return wire.BudgetResult{Refused: true, Reason: wire.ReasonNothingLatched, Message: "no budget limit is reached"}, nil
	}
	b.ledger.override(now)
	if err := b.ledger.save(now); err != nil {
		return wire.BudgetResult{}, err
	}
	return b.lifted(now), nil
}

// lifted is the tail of an override or increase: it ends a countdown the new
// state no longer needs, re-arms the notices and wakes the queue.
func (b *Budget) lifted(now time.Time) wire.BudgetResult {
	b.cancelHardGrace()
	status := b.Status(now)
	b.rearmNotes(status)
	b.events.Publish(wire.EventQueueChanged, "")
	b.guards.Kick()
	return wire.BudgetResult{Budget: &status}
}

// increase raises the live limits until the server restarts, then lifts
// each latch that spend since the override no longer reaches. A soft limit
// raised past the hard one takes the hard one with it, as config does.
func (b *Budget) increase(now time.Time, req wire.BudgetIncreaseRequest) (wire.BudgetResult, error) {
	if req.Soft == nil && req.Hard == nil {
		return wire.BudgetResult{Refused: true, Reason: wire.ReasonNotAnIncrease, Message: "name a soft or hard limit"}, nil
	}
	soft, hard := b.ledger.limits()
	for _, l := range []struct {
		name string
		cur  float64
		next *float64
	}{{"soft", soft, req.Soft}, {"hard", hard, req.Hard}} {
		switch {
		case l.next == nil:
		case l.cur == 0:
			return wire.BudgetResult{Refused: true, Reason: wire.ReasonNotAnIncrease, Message: l.name + " limit is off; any limit would lower it"}, nil
		case *l.next <= l.cur:
			return wire.BudgetResult{Refused: true, Reason: wire.ReasonNotAnIncrease, Message: fmt.Sprintf("%s limit is already %s", l.name, tickets.FormatCost(l.cur))}, nil
		}
	}
	if req.Soft != nil {
		soft = *req.Soft
	}
	if req.Hard != nil {
		hard = *req.Hard
	}
	if soft > 0 && hard > 0 && hard < soft {
		hard = soft
	}
	b.ledger.setLimits(now, soft, hard)
	if err := b.ledger.save(now); err != nil {
		return wire.BudgetResult{}, err
	}
	return b.lifted(now), nil
}

// iterationCostIn is the cumulative cost of a live iteration; ok is false when
// it has none (no session yet, an agent without a transcript cost, or no HOME
// in env to read the transcript under).
func iterationCostIn(env []string) func(wire.IterationInfo) (float64, bool) {
	return func(it wire.IterationInfo) (float64, bool) {
		if it.Session == "" {
			return 0, false
		}
		cost, ok, err := metrics.SessionCost(harness.LookupEnv(env, "HOME"), it.Worktree, it.Session)
		return cost, ok && err == nil
	}
}

// Iteration is the live iteration of address as its runner sees it, so a
// headless run is metered like a herdr pane.
func (b *Budget) Iteration(address string) (wire.IterationInfo, bool) {
	a, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return wire.IterationInfo{}, false
	}
	_, branch, worktree := worktree.IterationIdentity(a.Epic, a.ID, b.projects.WorktreeDir(a.Project))
	r, sess, found, err := b.harness.LiveSession(a)
	if err != nil || !found {
		return wire.IterationInfo{}, false
	}
	st, err := r.Status(sess)
	if err != nil {
		return wire.IterationInfo{}, false
	}
	return wire.IterationInfo{Address: address, Worktree: worktree, Branch: branch, Session: st.SessionID}, true
}

// poll adds every live iteration's cost delta to the ledger: claimed
// tickets, and paused ones whose pane a nudge set working (a watched run). All
// ticket kinds count.
func (b *Budget) poll(now time.Time) {
	for _, t := range b.idx.Snapshot().Tickets {
		if t.Status != "claimed" && !b.registry.Has(t.Address) {
			continue
		}
		// The ledger is dollars, and only a Claude run has a dollar cost.
		if run, found := b.registry.RunOf(t.Address); found && limits.IsCodexRun(run) {
			continue
		}
		it, ok := b.Iteration(t.Address)
		if !ok {
			continue
		}
		if cost, ok := b.costOf(it); ok {
			var project, root string
			if addr, err := tickets.ParseAddress(it.Address, tickets.AddressContext{}); err == nil {
				project, root = addr.Project, addr.Project+":"+addr.Epic
			}
			b.ledger.recordRoot(project, root, it.Address+"@"+it.Session, cost, now)
		}
	}
	hard := b.ledger.latches(now).Hard
	if err := b.ledger.save(now); err != nil {
		b.log.Error("save budget ledger", "err", err)
	}
	b.evs.observe(b.ledger.today(now))
	b.notify(now)
	if hard {
		b.killRuns(now, b.KillableRuns())
	}
}

// KillableRuns is the runs a budget kill stops: every Claude run with a
// session. A Codex run spends no dollars, so the limit never stops it.
func (b *Budget) KillableRuns() []runs.TrackedRun {
	var live []runs.TrackedRun
	for _, t := range b.registry.List() {
		// A launch has no session to interrupt yet; a later check stops it once it runs.
		if t.State != runs.Launching && !limits.IsCodexRun(t.Run) {
			live = append(live, t)
		}
	}
	return live
}

// killRuns stops every run in a list taken earlier: ctrl+c, a grace period,
// then it closes and pauses every pane budget-killed. Each run is marked first,
// so a run that settles on its own during the grace pauses budget-killed too
// (see settleRun) instead of finishing, merging or pausing for another reason.
// The list is the count the caller reports: a budget poll may stop and settle
// a run first.
func (b *Budget) killRuns(now time.Time, list []runs.TrackedRun) int {
	grace := b.cfg.KillGrace
	if grace <= 0 {
		grace = defaultKillGrace
	}
	deadline := now.Add(grace)
	var wg sync.WaitGroup
	for _, t := range list {
		b.registry.MarkBudgetStop(t.Address)
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := b.harness.RunnerOf(t.Run)
			if err == nil {
				err = r.Interrupt(t.Session)
			}
			if err != nil {
				b.log.Warn("budget stop signal", "ticket", t.Address, "err", err)
			}
		}()
	}
	wg.Wait()
	time.Sleep(time.Until(deadline))
	for _, t := range list {
		if _, first := b.registry.ClaimBudgetStop(t.Address); !first {
			continue // its run already settled and paused it
		}
		if err := b.PauseKilled(t); err != nil {
			b.log.Error("budget kill", "ticket", t.Address, "err", err)
		}
	}
	return len(list)
}

// PauseKilled stops the session through its runner, then pauses the ticket
// stuck. A run that settles or is nudged during a kill uses it too.
func (b *Budget) PauseKilled(t runs.TrackedRun) error {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	dir, _, err := b.projects.Of(addr.Project)
	if err != nil {
		return err
	}
	r, closeErr := b.harness.RunnerOf(t.Run)
	if closeErr == nil {
		closeErr = r.Stop(t.Session)
	}
	if err := tickets.EndOpenRound(t.TicketPath, tickets.SourceGx, tickets.HowAuto, ticketops.CloseBudgetKilled); err != nil {
		closeErr = errors.Join(closeErr, err)
	}
	pauseErr := b.host.PauseTicket(dir, addr, t.TicketPath, events.BudgetKilled, "daily budget hard limit reached", t.Run)
	return errors.Join(closeErr, pauseErr)
}

// KeepPolled polls the budget until ctx ends.
func (b *Budget) KeepPolled(ctx context.Context) {
	interval := b.cfg.PollInterval
	if interval <= 0 {
		interval = limits.DefaultPoll
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			b.poll(time.Now())
		}
	}
}

// StopEvents ends the budget-changed throttle.
func (b *Budget) StopEvents() { b.evs.stop() }
