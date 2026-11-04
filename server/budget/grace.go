package budget

import (
	"fmt"
	"sync"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

const defaultHardLimitGrace = 60 * time.Second

// hardGrace is the countdown a reload starts when it sets the hard limit at or
// below today's spend. The ledger's hold is what pauses starts and keeps the
// hard latch off; the countdown only decides when the hold ends. gen tells a
// timer that fired late from the one running now.
type hardGrace struct {
	mu    sync.Mutex
	timer *time.Timer
	until time.Time
	gen   int
}

// deadline is when the running countdown ends; zero when none runs.
func (g *hardGrace) deadline() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.until
}

// startHardGrace starts the countdown and announces it, unless one already
// runs: a further lowering reload does not buy more time.
func (b *Budget) startHardGrace(now time.Time, limit, spent float64) {
	g := &b.grace
	g.mu.Lock()
	if g.timer != nil {
		g.mu.Unlock()
		return
	}
	grace := b.cfg.HardLimitGrace
	if grace <= 0 {
		grace = defaultHardLimitGrace
	}
	g.gen++
	gen := g.gen
	g.until = now.Add(grace)
	g.timer = time.AfterFunc(grace, func() { b.endHardGrace(gen) })
	g.mu.Unlock()
	b.settings.Banner(wire.EventLevelWarning, HardGraceTitle(grace), fmt.Sprintf(
		"New hard budget limit (%s) is below today's spend (%s). %d agents will stop. Raise the limit in config.json or run `gx budget increase` to cancel.",
		tickets.FormatCost(limit), tickets.FormatCost(spent), len(b.registry.List())))
}

// HardGraceTitle counts whole-second durations in seconds, as the spec's
// "60s", where Duration.String would say "1m0s".
func HardGraceTitle(grace time.Duration) string {
	if grace%time.Second == 0 {
		return fmt.Sprintf("gx: agents stop in %ds", int(grace/time.Second))
	}
	return "gx: agents stop in " + grace.String()
}

// cancelHardGrace ends a running countdown once a raise has lifted the hold.
func (b *Budget) cancelHardGrace() {
	if b.ledger.held() || !b.StopHardGrace() {
		return
	}
	b.guards.Kick()
	b.settings.Banner(wire.EventLevelSuccess, "gx: agent stop cancelled — hard budget limit raised.", "")
}

// StopHardGrace stops the countdown and reports whether one was running.
func (b *Budget) StopHardGrace() bool {
	g := &b.grace
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.timer == nil {
		return false
	}
	g.timer.Stop()
	g.timer, g.until = nil, time.Time{}
	return true
}

// endHardGrace lifts the hold and checks again: spend still at the hard limit
// latches it and stops agents the way spend crossing it does. The reload lock
// keeps a lowering reload from starting a new countdown between the generation
// check and the latch; it is let go before the stop, which sleeps.
func (b *Budget) endHardGrace(gen int) {
	b.settings.Lock()
	g := &b.grace
	g.mu.Lock()
	if g.timer == nil || g.gen != gen {
		g.mu.Unlock()
		b.settings.Unlock()
		return
	}
	g.timer, g.until = nil, time.Time{}
	g.mu.Unlock()

	now := time.Now()
	// While held nothing is latched, so no budget poll has killed a run yet. Once
	// the hold lifts one may stop and settle a run before this stop looks.
	live := b.KillableRuns()
	// The banners below are this stop's report, so the hard notice is marked
	// sent before the hold lifts: a budget poll in between must not send one.
	b.notes.markHard()
	b.ledger.releaseHold()
	hard := b.ledger.latches(now).Hard
	if hard {
		if err := b.ledger.save(now); err != nil {
			b.log.Error("save budget ledger", "err", err)
		}
	} else {
		b.rearmNotes(b.Status(now))
	}
	b.settings.Unlock()
	b.events.Publish(wire.EventQueueChanged, "")
	if !hard {
		b.guards.Kick()
		return
	}
	n := b.killRuns(now, live)
	b.settings.Banner(wire.EventLevelError, fmt.Sprintf("gx: %d agents stopped — hard budget limit reached.", n), "")
}
