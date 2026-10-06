package ralphloop

import (
	"fmt"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// spinPolicy is the part of RunOptions the spin check needs.
type spinPolicy struct {
	cycles int
	window time.Duration
}

// newSpinPolicy fills unset (zero) options from the config defaults, the only
// place those defaults live.
func newSpinPolicy(opts RunOptions) spinPolicy {
	def := config.DefaultExecutionQueueConfig()
	p := spinPolicy{cycles: opts.SpinCycles, window: opts.SpinWindow}
	if p.cycles < 1 {
		p.cycles = def.SpinCycles
	}
	if p.window <= 0 {
		p.window = def.SpinWindow
	}
	return p
}

// quarantined reports whether ticket id has parked at least cycles times
// inside the window ending at now. Every park is followed by a re-claim (that
// is how the ticket got back to the scheduler), so the park count is the cycle
// count; counting from the run log keeps it across a restart.
func (p spinPolicy) quarantined(log []Event, id string, now time.Time) bool {
	n := 0
	for _, ev := range log {
		if ev.Ticket != id || now.Sub(ev.Time) > p.window {
			continue
		}
		if isParkType(events.Type(ev.Type)) && ev.Kind != string(events.Spinning) {
			n++
		}
	}
	return n >= p.cycles
}

// firstNonSpinning returns the first frontier candidate that is not skipped and
// not spinning, quarantining each spinning one it passes as needs-repair/spinning.
// It reads the run log at most once, however many candidates there are: the
// parks it appends are Kind spinning, which the count ignores.
func firstNonSpinning(opts RunOptions, now time.Time, sink EventSink, scratchDir, wtDir string, frontier []tickets.Ticket, skip func(tickets.Ticket) bool) (tickets.Ticket, error) {
	policy := newSpinPolicy(opts)
	var log []Event
	read := false
	for _, t := range frontier {
		if skip(t) {
			continue
		}
		if !read {
			var ok bool
			var err error
			log, ok, err = ReadEvents(scratchDir, opts.EpicName)
			if err != nil {
				return tickets.Ticket{}, fmt.Errorf("reading run log for spin check: %w", err)
			}
			if !ok {
				log = nil
			}
			read = true
		}
		if !policy.quarantined(log, t.Identifier, now) {
			return t, nil
		}
		_, err := park(sink, parkRequest{
			ScratchDir: scratchDir, EpicName: opts.EpicName, Ticket: t.Identifier, Path: t.Path,
			Type: events.NeedsRepair, Kind: events.Spinning,
			Reason: fmt.Sprintf("parked and re-claimed %d times within %s", policy.cycles, policy.window),
			Repair: schema.NeedsRepairState{
				Label:    iterLabel(opts.EpicName, t.Identifier),
				Branch:   iterBranch(opts.EpicName, t.Identifier),
				Worktree: iterationWorktreePath(wtDir, opts.EpicName, t.Identifier),
			},
		})
		if err != nil {
			return tickets.Ticket{}, err
		}
	}
	return tickets.Ticket{}, nil
}
