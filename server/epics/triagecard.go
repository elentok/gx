package epics

import (
	"slices"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/triage"
)

const (
	// PillWaitingForYou marks a Triage that needs the person; PillStopped one with no session.
	PillWaitingForYou = "Waiting for you"
	PillStopped       = "Stopped"
)

// triageSteps are the five steps of the ring, in order; Done is the last.
var triageSteps = []string{tickets.StepVerify, tickets.StepPlan, tickets.StepApprove, tickets.StepWrite, "done"}

// TriagePlacement is where the Follow-ups card sits and what it shows. A zero
// Section means the plain card: no live Triage.
type TriagePlacement struct {
	Section wire.CardSection
	Pill    string
	Action  wire.CardAction
}

// PlaceTriage places the Follow-ups card of project from its live record (nil
// or not live: none) and whether a live session runs the Triage.
func PlaceTriage(project string, rec *triage.Record, sessionLive bool) TriagePlacement {
	link := "#/triage/" + project
	plain := TriagePlacement{
		Section: wire.SectionReadyToDesign,
		Action:  wire.CardAction{Kind: wire.CardActionTriage, Label: "Triage", Target: project},
	}
	if rec == nil || rec.State != triage.StateLive {
		return plain
	}
	needsYou := func(label string) TriagePlacement {
		return TriagePlacement{Section: wire.SectionNeedsYou, Pill: PillWaitingForYou,
			Action: wire.CardAction{Kind: wire.CardActionOpen, Label: label, Target: project, Link: link}}
	}
	switch {
	case rec.Step == tickets.StepApprove:
		return needsYou("Review plan")
	case sessionLive:
		return TriagePlacement{Section: wire.SectionDesigning,
			Action: wire.CardAction{Kind: wire.CardActionOpen, Label: "Open", Target: project, Link: link}}
	}
	stopped := TriagePlacement{Section: wire.SectionDesigning, Pill: PillStopped,
		Action: wire.CardAction{Kind: wire.CardActionContinue, Label: "Continue", Target: project}}
	if rec.Step != tickets.StepWrite {
		return stopped
	}
	failed := false
	for _, r := range rec.Rows {
		switch r.Write {
		case triage.WritePending:
			return stopped
		case triage.WriteFailed:
			failed = true
		}
	}
	if !rec.LastPassComplete || !failed {
		return stopped
	}
	return needsYou("Review failures")
}

// triagePercent is the ring: the steps passed plus the share of this step's rows
// handled, over the four steps before Done.
func triagePercent(rec *triage.Record) int {
	if rec.State == triage.StateDone {
		return 100
	}
	idx := slices.Index(triageSteps, rec.Step)
	if idx < 0 || len(rec.Rows) == 0 {
		return max(idx, 0) * 25
	}
	handled := 0
	for _, r := range rec.Rows {
		switch rec.Step {
		case tickets.StepVerify:
			if r.Verdict != "" {
				handled++
			}
		case tickets.StepPlan:
			if r.Plan != nil {
				handled++
			}
		case tickets.StepWrite:
			if r.Write == triage.WriteDone || r.Write == triage.WriteGone {
				handled++
			}
		}
	}
	return min(100, (idx*len(rec.Rows)+handled)*25/len(rec.Rows))
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
