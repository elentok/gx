package schedules

import (
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
func (sc *Scheduler) dedupeKey(k Key) string {
	return "schedule:" + k.Project + "/" + k.Slug + "@" + sc.State.MachineID()
}

// previousRun finds the live one-off carrying the Schedule's Dedupe key.
func (sc *Scheduler) previousRun(k Key) (*tickets.Ticket, string, error) {
	live, _, err := tickets.LoadOneOffs(filepath.Join(sc.TicketStore, k.Project))
	if err != nil {
		return nil, "", err
	}
	key := sc.dedupeKey(k)
	for i := range live.Tickets {
		t := live.Tickets[i]
		if t.Unique == key && !tickets.IsTerminalStatus(t.Status) {
			addr := tickets.Address{Project: k.Project, Epic: tickets.OneOffsEpic, ID: t.DisplayNumber()}.String()
			return &t, addr, nil
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

// prepare applies the decision for a timed run: it cancels a replaceable
// run and reports whether the run may start. A skip is logged here.
// retry means the baseline must stay.
func (sc *Scheduler) prepare(k Key, due, now time.Time) (ok, retry bool) {
	d, err := sc.Decision(k)
	if err != nil {
		sc.Log.Warn("finding the previous run failed", "schedule", k.String(), "err", err)
		return false, true
	}
	if d.Action == ActionReplace {
		res, err := sc.Cancel(wire.QueueRequest{Address: d.Address, Stop: true})
		if err != nil || res.Refused {
			// It may have started in between; look again and skip if so.
			if d, err = sc.Decision(k); err != nil || d.Action != ActionSkip {
				sc.Log.Warn("cancelling the previous run failed", "schedule", k.String(), "err", err)
				return false, true
			}
		} else {
			return true, false
		}
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
