package ralphloop

import (
	"fmt"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const (
	defaultSpinCycles = 3
	defaultSpinWindow = 5 * time.Minute
)

// spinQuarantined reports whether ticket id has parked at least cycles times
// inside window ending at now. Every park is followed by a re-claim (that is
// how the ticket got back to the scheduler), so the park count is the cycle
// count; counting from the run log keeps it across a restart.
func spinQuarantined(log []Event, id string, now time.Time, cycles int, window time.Duration) bool {
	n := 0
	for _, ev := range log {
		if ev.Ticket != id || now.Sub(ev.Time) > window {
			continue
		}
		if (ev.Type == string(events.NeedsAnswer) || ev.Type == string(events.NeedsRepair)) && ev.Kind != string(events.Spinning) {
			n++
		}
	}
	return n >= cycles
}

// quarantineIfSpinning parks t as needs-repair/spinning when it is about to be
// re-claimed after too many recent parks, and reports whether it did.
func quarantineIfSpinning(opts RunOptions, now time.Time, sink EventSink, scratchDir string, t tickets.Ticket, repair schema.NeedsRepairState) (bool, error) {
	log, ok, err := ReadEvents(scratchDir, opts.EpicName)
	if err != nil {
		return false, fmt.Errorf("reading run log for spin check: %w", err)
	}
	if !ok {
		return false, nil
	}
	cycles, window := opts.SpinCycles, opts.SpinWindow
	if cycles < 1 {
		cycles = defaultSpinCycles
	}
	if window <= 0 {
		window = defaultSpinWindow
	}
	if !spinQuarantined(log, t.Identifier, now, cycles, window) {
		return false, nil
	}
	_, err = park(sink, parkRequest{
		ScratchDir: scratchDir, EpicName: opts.EpicName, Ticket: t.Identifier, Path: t.Path,
		Type: events.NeedsRepair, Kind: events.Spinning,
		Reason: fmt.Sprintf("parked and re-claimed %d times within %s", cycles, window),
		Repair: repair,
	})
	return true, err
}
