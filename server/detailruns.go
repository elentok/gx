package server

import (
	"os"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Outcomes of a run line.
const (
	RunMerged          = "merged"
	RunMergedNoCommits = "merged-no-commits"
	RunPaused          = "paused"
	RunStartedOver     = "started-over"
	RunRunning         = "running"
	RunAgentLost       = "agent-lost"
	// RunAgentGoneAfterRestart is a run a restarted server found dead with no
	// work to keep, and put back in the queue.
	RunAgentGoneAfterRestart = "agent-gone-after-restart"
	RunRelaunched            = "relaunched"
	RunCancelled             = "cancelled"
)

// How a run began. The run log keeps cold-start and resumed; the page labels
// them "new session" and "same session".
const (
	StartNewSession  = "new-session"
	StartSameSession = "same-session"
)

// DetailRun is one run line of the Runs section.
type DetailRun struct {
	Start time.Time `json:"start"`
	// Duration is in seconds; a running or lost run has none.
	Duration int    `json:"duration,omitempty"`
	Outcome  string `json:"outcome"`
	// PauseLabel is the reason label of a run that ended in a pause.
	PauseLabel string `json:"pause_label,omitempty"`
	StartKind  string `json:"start_kind"`
}

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
func detailRuns(src runSources) []DetailRun {
	b := runBuilder{runs: []DetailRun{}}
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
	runs []DetailRun
	open bool
	// answeredAt is when the last pause was answered, until a run takes it up.
	answeredAt time.Time
}

func (b *runBuilder) begin(at time.Time, kind string) {
	b.runs = append(b.runs, DetailRun{Start: at, StartKind: kind})
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
	if !b.open && len(b.runs) > 0 && b.runs[len(b.runs)-1].Outcome == RunPaused && b.answeredAt.IsZero() {
		b.answeredAt = at
	}
}

func (b *runBuilder) event(ev ralphloop.Event) {
	switch events.Type(ev.Type) {
	case events.IterationStarted:
		if b.open {
			b.end(ev.Time, RunAgentLost)
		}
		kind := StartNewSession
		if ev.Path == "resumed" {
			kind = StartSameSession
		}
		b.begin(ev.Time, kind)
	case events.ClaimReopened:
		b.reopen(ev.Time, RunAgentGoneAfterRestart)
	case events.HumanRelaunch:
		b.reopen(ev.Time, RunRelaunched)
	case events.LateFinishAdopted:
		if !b.open {
			b.begin(ev.Time, StartSameSession)
		}
	default:
		outcome := runEnd(ev)
		if outcome == "" {
			return
		}
		if !b.open && !b.answeredAt.IsZero() {
			b.begin(b.answeredAt, StartSameSession)
		}
		if !b.open {
			return
		}
		b.end(ev.Time, outcome)
		if outcome == RunPaused {
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
		b.begin(b.answeredAt, StartSameSession)
	}
	if !b.open {
		return
	}
	last := &b.runs[len(b.runs)-1]
	switch t.Status {
	case schema.StatusCancelled:
		b.end(cancelledAt, RunCancelled)
	case schema.StatusClaimed:
		last.Outcome = RunRunning
	default:
		last.Outcome = RunAgentLost
	}
}

// runEnd is the outcome an event ends a run with, or "" for an event that does not.
func runEnd(ev ralphloop.Event) string {
	switch events.Type(ev.Type) {
	case events.CherryPicked, events.ManualMerge:
		return RunMerged
	case events.Commitless:
		return RunMergedNoCommits
	case events.Paused:
		return RunPaused
	case events.TicketStartedOver:
		return RunStartedOver
	}
	return ""
}

// ticketRuns reads the ticket's run lines from its epic's run log.
func (s *Server) ticketRuns(ref ticketRef, t schema.Ticket, log []tickets.LogEvent, statusWrittenAt time.Time) ([]DetailRun, error) {
	evs, err := s.readHistory(ref.addr)
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
