package server

import (
	"github.com/elentok/gx/server/wire"
	"os"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// runSources is everything the run lines of one ticket are built from.
type runSources struct {
	// events is the ticket's run-log events, oldest first.
	events []ralphloop.Event
	// answers are the times of the ticket log's pause answers, oldest first.
	answers []time.Time
	// ticket is the ticket as stored; its status says whether an open last run
	// is still running or was cancelled.
	ticket schema.Ticket
	// statusWrittenAt is when the ticket file was last written, which is when a
	// cancel wrote its status.
	statusWrittenAt time.Time
}

// detailRuns turns a ticket's run-log events into run lines. A run opens at
// iteration-started and ends at the next merge, pause or start over. A run
// with no such end is closed by claim-reopened, human-relaunch, the next
// iteration-started or a cancel. Two things open a run with no
// iteration-started: a late-finish-adopted event, and a pause answer that
// claimed the ticket again for the live agent. A reclaim writes no run-log
// line, so it never splits a run.
func detailRuns(src runSources) []wire.DetailRun {
	b := runBuilder{runs: []wire.DetailRun{}}
	answers := src.answers
	for _, ev := range src.events {
		// An answer at the same instant as an event comes first.
		for len(answers) > 0 && !answers[0].After(ev.Time) {
			b.answer(answers[0])
			answers = answers[1:]
		}
		b.event(ev)
	}
	for _, at := range answers {
		b.answer(at)
	}
	b.finish(src.ticket, src.statusWrittenAt)
	return b.runs
}

// runBuilder folds events and pause answers, oldest first, into run lines.
type runBuilder struct {
	runs []wire.DetailRun
	open bool
	// answeredAt is when the last pause was answered, until a run takes it up.
	answeredAt time.Time
}

func (b *runBuilder) begin(at time.Time, kind string) {
	b.runs = append(b.runs, wire.DetailRun{Start: at, StartKind: kind})
	b.open = true
	b.answeredAt = time.Time{}
}

func (b *runBuilder) end(at time.Time, outcome string) {
	last := &b.runs[len(b.runs)-1]
	last.Outcome = outcome
	last.Duration = int(at.Sub(last.Start).Seconds())
	b.open = false
}

// reopen ends an open run with outcome and forgets a pending answer: the ticket
// was put back in the queue, so the answer no longer starts a run.
func (b *runBuilder) reopen(at time.Time, outcome string) {
	if b.open {
		b.end(at, outcome)
	}
	b.answeredAt = time.Time{}
}

// answer notes a pause answer; only the first one after a pause counts.
func (b *runBuilder) answer(at time.Time) {
	if !b.open && len(b.runs) > 0 && b.runs[len(b.runs)-1].Outcome == wire.RunPaused && b.answeredAt.IsZero() {
		b.answeredAt = at
	}
}

func (b *runBuilder) event(ev ralphloop.Event) {
	switch events.Type(ev.Type) {
	case events.IterationStarted:
		if b.open {
			b.end(ev.Time, wire.RunAgentLost)
		}
		kind := wire.StartNewSession
		if ev.Path == "resumed" {
			kind = wire.StartSameSession
		}
		b.begin(ev.Time, kind)
	case events.ClaimReopened:
		b.reopen(ev.Time, wire.RunAgentGoneAfterRestart)
	case events.HumanRelaunch:
		b.reopen(ev.Time, wire.RunRelaunched)
	case events.LateFinishAdopted:
		if !b.open {
			b.begin(ev.Time, wire.StartSameSession)
		}
	default:
		outcome := runEnd(ev)
		if outcome == "" {
			return
		}
		if !b.open && !b.answeredAt.IsZero() {
			b.begin(b.answeredAt, wire.StartSameSession)
		}
		if !b.open {
			return
		}
		b.end(ev.Time, outcome)
		if outcome == wire.RunPaused {
			b.runs[len(b.runs)-1].PauseLabel = schema.PauseReason(ev.Kind).Label()
		}
	}
}

// finish settles the last run from the ticket as stored. An answer after the
// last run-log event claimed the ticket again only if the claim is newer than
// the answer. cancelledAt is when a cancelled ticket's status was written.
func (b *runBuilder) finish(t schema.Ticket, cancelledAt time.Time) {
	if !b.open && !b.answeredAt.IsZero() && !t.ClaimedAt.Before(b.answeredAt) &&
		(t.Status == schema.StatusClaimed || t.Status == schema.StatusCancelled) {
		b.begin(b.answeredAt, wire.StartSameSession)
	}
	if !b.open {
		return
	}
	last := &b.runs[len(b.runs)-1]
	switch t.Status {
	case schema.StatusCancelled:
		b.end(cancelledAt, wire.RunCancelled)
	case schema.StatusClaimed:
		last.Outcome = wire.RunRunning
	default:
		last.Outcome = wire.RunAgentLost
	}
}

// runEnd is the outcome an event ends a run with, or "" for an event that does not.
func runEnd(ev ralphloop.Event) string {
	switch events.Type(ev.Type) {
	case events.CherryPicked, events.ManualMerge:
		return wire.RunMerged
	case events.Commitless:
		return wire.RunMergedNoCommits
	case events.Paused:
		return wire.RunPaused
	case events.TicketStartedOver:
		return wire.RunStartedOver
	}
	return ""
}

// ticketRuns reads the ticket's run lines from its epic's run log.
func (s *Server) ticketRuns(ref ticketRef, t schema.Ticket, log []tickets.LogEvent, statusWrittenAt time.Time) ([]wire.DetailRun, error) {
	evs, err := s.idx.ReadHistory(ref.addr)
	if err != nil {
		return nil, err
	}
	src := runSources{events: evs, ticket: t, statusWrittenAt: statusWrittenAt}
	for _, ev := range log {
		if ev.Type == tickets.EventPauseAnswer {
			src.answers = append(src.answers, ev.TS)
		}
	}
	return detailRuns(src), nil
}

// ticketWrittenAt is when the ticket file was last written, or zero if it
// cannot be read.
func ticketWrittenAt(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}
