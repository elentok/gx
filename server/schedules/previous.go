package schedules

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Action is what a run request does about the previous run of its Schedule.
type Action string

const (
	ActionStart Action = "start"
	// ActionReplace cancels a queued, never-started previous run, then starts.
	ActionReplace Action = "replace"
	// ActionSkip is a timed run that does nothing; Run now refuses instead.
	ActionSkip Action = "skip"
)

// Decision is the answer to "may a run start now". It has no side effects;
// the caller performs the cancel and the submit.
type Decision struct {
	Action Action
	// Reason is previous-run-started or previous-run-needs-attention for a skip.
	Reason string
	// Address is the previous run's ticket, set for replace and for a skip.
	Address string
}

// Decide looks at the previous run's ticket (nil when none is live).
func Decide(prev *tickets.Ticket, address string) Decision {
	switch {
	case prev == nil, tickets.IsTerminalStatus(prev.Status):
		return Decision{Action: ActionStart}
	case prev.Status == "open" && prev.ClaimedAt.IsZero():
		return Decision{Action: ActionReplace, Address: address}
	case prev.Status == "paused":
		return Decision{Action: ActionSkip, Reason: ReasonPreviousRunNeedsAttention, Address: address}
	default:
		return Decision{Action: ActionSkip, Reason: ReasonPreviousRunStarted, Address: address}
	}
}

// dedupeKey is the Dedupe key of this machine's runs of the Schedule.
func (sc *Scheduler) dedupeKey(k Key) string { return DedupeKey(k, sc.State.MachineID()) }

// DedupeKey is the Dedupe key of one machine's runs of the Schedule.
func DedupeKey(k Key, machineID string) string {
	return "schedule:" + k.String() + "@" + machineID
}

// oneOff is a live One-off with its address.
type oneOff struct {
	Ticket  tickets.Ticket
	Address string
}

// oneOffsOf lists this machine's live One-offs carrying the Schedule's Dedupe key.
func (sc *Scheduler) oneOffsOf(k Key) ([]oneOff, error) {
	live, _, err := tickets.LoadOneOffs(filepath.Join(sc.TicketStore, k.Project))
	if err != nil {
		return nil, err
	}
	key := sc.dedupeKey(k)
	var out []oneOff
	for _, t := range live.Tickets {
		if t.Unique == key {
			addr := tickets.Address{Project: k.Project, Epic: tickets.OneOffsEpic, ID: t.DisplayNumber()}.String()
			out = append(out, oneOff{Ticket: t, Address: addr})
		}
	}
	return out, nil
}

// previousRun finds the live one-off carrying the Schedule's Dedupe key.
func (sc *Scheduler) previousRun(k Key) (*tickets.Ticket, string, error) {
	runs, err := sc.oneOffsOf(k)
	if err != nil {
		return nil, "", err
	}
	for i := range runs {
		if !tickets.IsTerminalStatus(runs[i].Ticket.Status) {
			return &runs[i].Ticket, runs[i].Address, nil
		}
	}
	return nil, "", nil
}

// Decision reports what a run of the Schedule would do now. Run now calls it too.
func (sc *Scheduler) Decision(k Key) (Decision, error) {
	prev, addr, err := sc.previousRun(k)
	if err != nil {
		return Decision{}, err
	}
	return Decide(prev, addr), nil
}

// settlePrevious decides what a run does about the previous run and cancels a
// replaceable one. The result is ActionStart or ActionSkip; an error means the
// previous run could not be found or cancelled.
func (sc *Scheduler) settlePrevious(k Key) (Decision, error) {
	d, err := sc.Decision(k)
	if err != nil || d.Action != ActionReplace {
		return d, err
	}
	res, cerr := sc.Cancel(wire.QueueRequest{Address: d.Address, Stop: true})
	if cerr == nil && !res.Refused {
		return Decision{Action: ActionStart}, nil
	}
	// It may have started in between; look again.
	if d, err = sc.Decision(k); err != nil {
		return Decision{}, err
	}
	if d.Action == ActionSkip {
		return d, nil
	}
	if cerr == nil {
		cerr = fmt.Errorf("cancelling the previous run was refused (%s): %s", res.Reason, res.Message)
	}
	return Decision{}, cerr
}

// prepare applies the decision for a timed run and reports whether the run
// may start. A skip is logged here. retry means the baseline must stay.
func (sc *Scheduler) prepare(k Key, due, now time.Time) (ok, retry bool) {
	d, err := sc.settlePrevious(k)
	if err != nil {
		sc.Log.Warn("settling the previous run failed", "schedule", k.String(), "err", err)
		return false, true
	}
	if d.Action == ActionSkip {
		sk := Skip{SetTime: due, Reason: d.Reason, LoggedAt: now}
		if d.Reason == ReasonPreviousRunNeedsAttention {
			sk.Blocking = d.Address
		}
		return false, !sc.record(sc.State.AppendSkip(k, sk), k)
	}
	return true, false
}
