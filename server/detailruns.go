package server

import (
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

// detailRuns turns a ticket's run-log events (oldest first) into run lines. A
// run opens at iteration-started and ends at the next merge, pause or start
// over; every other event adds no line. claimed says whether an open last run
// is still running.
func detailRuns(evs []ralphloop.Event, claimed bool) []DetailRun {
	runs := []DetailRun{}
	open := false
	for _, ev := range evs {
		if ev.Type == string(events.IterationStarted) {
			kind := StartNewSession
			if ev.Path == "resumed" {
				kind = StartSameSession
			}
			runs = append(runs, DetailRun{Start: ev.Time, StartKind: kind})
			open = true
			continue
		}
		outcome := runEnd(ev)
		if outcome == "" || !open {
			continue
		}
		last := &runs[len(runs)-1]
		last.Outcome = outcome
		last.Duration = int(ev.Time.Sub(last.Start).Seconds())
		if outcome == RunPaused {
			last.PauseLabel = schema.PauseReason(ev.Kind).Label()
		}
		open = false
	}
	if open {
		last := &runs[len(runs)-1]
		last.Outcome = RunAgentLost
		if claimed {
			last.Outcome = RunRunning
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
func (s *Server) ticketRuns(addr tickets.Address, t schema.Ticket) ([]DetailRun, error) {
	evs, err := s.readHistory(addr)
	if err != nil {
		return nil, err
	}
	return detailRuns(evs, t.Status == schema.StatusClaimed), nil
}
