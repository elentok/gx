package epics

import (
	"fmt"
	"slices"
	"strings"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/triage"
)

// PillNeedsYou marks a Triage that waits for the person's answer or approval.
const PillNeedsYou = "Needs you"

// triageSteps are the five steps of the ring, in order; Done is the last.
var triageSteps = []string{tickets.StepVerify, tickets.StepRoute, tickets.StepQuestions, tickets.StepWrite, "done"}

// TriagePlacement is where the Follow-ups card sits and what it shows. A zero
// Section means the plain card: no live Triage.
type TriagePlacement struct {
	Section wire.CardSection
	// Pill is the step name, or "Needs you"; empty on the plain card.
	Pill string
	// Line is the second line under the pill.
	Line string
	// RetryFailed is the number of failed rows a Write offers to retry.
	RetryFailed int
	// Stopped: a live Triage with no session that is not waiting for the person.
	Stopped bool
	Action  wire.CardAction
}

// PlaceTriage places the Follow-ups card of project from its live record (nil
// or not live: none) and whether a live session runs the Triage.
func PlaceTriage(project string, rec *triage.Record, sessionLive bool) TriagePlacement {
	link := "#/triage/" + project
	if rec == nil || rec.State != triage.StateLive {
		return TriagePlacement{
			Section: wire.SectionReadyToDesign,
			Action:  wire.CardAction{Kind: wire.CardActionTriage, Label: "Triage", Target: project},
		}
	}
	open := func(label string) wire.CardAction {
		return wire.CardAction{Kind: wire.CardActionOpen, Label: label, Target: project, Link: link}
	}
	p := TriagePlacement{
		Section: wire.SectionDesigning,
		Action:  wire.CardAction{Kind: wire.CardActionContinue, Label: "Continue", Target: project},
	}
	switch rec.Step {
	case tickets.StepVerify:
		handled := 0
		for _, r := range rec.Rows {
			if r.Verdict != "" {
				handled++
			}
		}
		p.Pill, p.Line = "Verify", fmt.Sprintf("Verifying %d of %d", handled, len(rec.Rows))
	case tickets.StepRoute:
		p.Pill, p.Line = "Route", "Routing"
	case tickets.StepQuestions:
		p.Pill, p.Line = "Questions", "Questions"
		if idx, qs, ok := rec.OpenRound(); ok {
			p.Section, p.Pill, p.Action = wire.SectionNeedsYou, PillNeedsYou, open("Answer")
			p.Line = fmt.Sprintf("Round %d · %d questions left", idx+1, len(qs))
		} else if len(rec.Unrounded()) == 0 {
			p.Section, p.Pill, p.Action = wire.SectionNeedsYou, PillNeedsYou, open("Review plan")
			p.Line = "Write this plan?"
		}
	case tickets.StepWrite:
		written, failed, pending := 0, 0, false
		for _, r := range rec.Rows {
			if rowWritten(r) {
				written++
				continue
			}
			switch r.Write {
			case triage.WriteFailed:
				failed++
			case triage.WritePending:
				pending = true
			}
		}
		p.Pill, p.Line, p.RetryFailed = "Write", fmt.Sprintf("%d of %d written", written, len(rec.Rows)), failed
		if !sessionLive && !pending && rec.LastPassComplete && failed > 0 {
			p.Section = wire.SectionNeedsYou
		}
	}
	p.Stopped = !sessionLive && p.Section != wire.SectionNeedsYou
	return p
}

// donePlacement is the Done card of a finished Triage whose ready epic holds
// tickets that are not queued yet.
func donePlacement(readyEpic string, ready int) TriagePlacement {
	return TriagePlacement{
		Section: wire.SectionReadyToDesign, Pill: "Done", Line: fmt.Sprintf("%d ready to queue", ready),
		Action: wire.CardAction{Kind: wire.CardActionQueue, Label: "Queue", Target: readyEpic},
	}
}

// readyToQueue is the number of open tickets of the done record's ready epic
// while that epic is not in the queue; 0 means the plain card.
func readyToQueue(project string, done *triage.Record, epics []tickets.Epic, qidx queueIndex) int {
	if done == nil || done.State != triage.StateDone || done.ReadyEpic == "" {
		return 0
	}
	_, name, ok := tickets.ParseEpicAddress(done.ReadyEpic)
	if !ok || qidx.slot(queue.RootRef{Project: project, Epic: name}.String()) != 0 {
		return 0
	}
	n := 0
	for _, e := range epics {
		if e.Name != name {
			continue
		}
		for _, t := range e.Tickets {
			if t.Status == string(schema.StatusOpen) {
				n++
			}
		}
	}
	return n
}

// rowWritten is a row the Write step has finished with: written, gone or kept
// for later. It matches the Progress ring's written count.
func rowWritten(r triage.Row) bool {
	return r.Write == triage.WriteDone || r.Write == triage.WriteGone || r.Person.KeepForLater
}

// triagePercent is the ring: the steps passed plus the share of this step's rows
// handled, over the four steps before Done. The Questions share is the answered
// share of the open round.
func triagePercent(rec *triage.Record) int {
	if rec.State == triage.StateDone {
		return 100
	}
	step := rec.Step
	idx := slices.Index(triageSteps, step)
	handled, total := 0, len(rec.Rows)
	if step == tickets.StepQuestions {
		handled, total = 0, 0
		if i, _, ok := rec.OpenRound(); ok {
			for _, q := range rec.Rounds[i].Questions {
				total++
				if strings.TrimSpace(rec.Rounds[i].Answers[q.ID]) != "" {
					handled++
				}
			}
		} else if len(rec.Rounds) > 0 {
			handled, total = 1, 1
		}
	} else {
		for _, r := range rec.Rows {
			switch step {
			case tickets.StepVerify:
				if r.Verdict != "" {
					handled++
				}
			case tickets.StepRoute:
				if r.Plan != nil {
					handled++
				}
			case tickets.StepWrite:
				if rowWritten(r) {
					handled++
				}
			}
		}
	}
	if idx < 0 || total == 0 {
		return max(idx, 0) * 25
	}
	return min(100, (idx*total+handled)*25/total)
}

// triageSessions folds the sessions into the projects with a live Triage
// session: flow triage, not exited, no failure.
func (s *Epics) triageSessions() map[string]bool {
	out := map[string]bool{}
	sessions, err := s.Sessions.ListSessions()
	if err != nil {
		s.Log.Warn("follow-ups card: list sessions", "err", err)
		return out
	}
	for _, info := range sessions {
		if info.Flow == string(tickets.FlowTriage) && info.Status.State != wire.SessionExited && info.Failure == nil {
			out[info.FlowKey] = true
		}
	}
	return out
}
