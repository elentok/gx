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
	runs := []DetailRun{}
	open := false
	// answeredAt is when the last pause was answered, until a run takes it up.
	var answeredAt time.Time
	begin := func(at time.Time, kind string) {
		runs = append(runs, DetailRun{Start: at, StartKind: kind})
		open = true
		answeredAt = time.Time{}
	}
	end := func(at time.Time, outcome string) {
		last := &runs[len(runs)-1]
		last.Outcome = outcome
		last.Duration = int(at.Sub(last.Start).Seconds())
		open = false
	}
	answers := src.answers
	// noteAnswers takes in the answers up to until; only the first one after a
	// pause counts.
	noteAnswers := func(until func(time.Time) bool) {
		for len(answers) > 0 && until(answers[0]) {
			if !open && len(runs) > 0 && runs[len(runs)-1].Outcome == RunPaused && answeredAt.IsZero() {
				answeredAt = answers[0]
			}
			answers = answers[1:]
		}
	}
	for _, ev := range src.events {
		noteAnswers(func(at time.Time) bool { return !at.After(ev.Time) })
		switch events.Type(ev.Type) {
		case events.IterationStarted:
			if open {
				end(ev.Time, RunAgentLost)
			}
			kind := StartNewSession
			if ev.Path == "resumed" {
				kind = StartSameSession
			}
			begin(ev.Time, kind)
		case events.ClaimReopened:
			if open {
				end(ev.Time, RunAgentGoneAfterRestart)
			}
			answeredAt = time.Time{}
		case events.HumanRelaunch:
			if open {
				end(ev.Time, RunRelaunched)
			}
			answeredAt = time.Time{}
		case events.LateFinishAdopted:
			if !open {
				begin(ev.Time, StartSameSession)
			}
		default:
			outcome := runEnd(ev)
			if outcome == "" {
				continue
			}
			if !open && !answeredAt.IsZero() {
				begin(answeredAt, StartSameSession)
			}
			if !open {
				continue
			}
			end(ev.Time, outcome)
			if outcome == RunPaused {
				runs[len(runs)-1].PauseLabel = schema.PauseReason(ev.Kind).Label()
			}
		}
	}
	// An answer after the last run-log event claimed the ticket again only if
	// the claim is newer than the answer.
	noteAnswers(func(time.Time) bool { return true })
	if !open && !answeredAt.IsZero() && !src.ticket.ClaimedAt.Before(answeredAt) &&
		(src.ticket.Status == schema.StatusClaimed || src.ticket.Status == schema.StatusCancelled) {
		begin(answeredAt, StartSameSession)
	}
	if open {
		switch src.ticket.Status {
		case schema.StatusCancelled:
			end(src.statusWrittenAt, RunCancelled)
		case schema.StatusClaimed:
			runs[len(runs)-1].Outcome = RunRunning
		default:
			runs[len(runs)-1].Outcome = RunAgentLost
		}
	}
	return runs
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
func (s *Server) ticketRuns(ref ticketRef, t schema.Ticket, log []tickets.LogEvent) ([]DetailRun, error) {
	evs, err := s.readHistory(ref.addr)
	if err != nil {
		return nil, err
	}
	src := runSources{events: evs, ticket: t}
	for _, ev := range log {
		if ev.Type == tickets.EventPauseAnswer {
			src.answers = append(src.answers, ev.TS)
		}
	}
	if t.Status == schema.StatusCancelled {
		if fi, err := os.Stat(ref.ticket.Path); err == nil {
			src.statusWrittenAt = fi.ModTime()
		}
	}
	return detailRuns(src), nil
}
