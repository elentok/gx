package triage

import "github.com/elentok/gx/tickets"

// Branch is what the skill does next.
type Branch string

const (
	BranchStart     Branch = "start"
	BranchVerify    Branch = "verify"
	BranchRoute     Branch = "route"
	BranchQuestions Branch = "questions"
	BranchWrite     Branch = "write"
	BranchDone      Branch = "done"
)

// Next detects the step from the record: no live record starts one, else rows
// with no verdict (verify), no saved plan (route), a saved plan not yet
// approved (questions), approved and not done (write), done.
func Next(rec *Record) Branch {
	if rec == nil || rec.State == StateAbandoned {
		return BranchStart
	}
	if rec.State == StateDone {
		return BranchDone
	}
	switch rec.Step {
	case tickets.StepWrite:
		return BranchWrite
	case tickets.StepQuestions:
		return BranchQuestions
	}
	if len(rec.PendingVerdicts()) > 0 {
		return BranchVerify
	}
	return BranchRoute
}

// Final reports whether no run follows the record's step: no live record, the
// questions and done steps, and a write with no row or move unit left pending.
// Verify and route are not final; route's last save moves the step to
// questions before its run ends.
func Final(rec *Record) bool {
	if rec == nil || rec.State != StateLive {
		return true
	}
	switch rec.Step {
	case tickets.StepVerify, tickets.StepRoute:
		return false
	case tickets.StepWrite:
		for _, row := range rec.Rows {
			if row.Write == WritePending {
				return false
			}
		}
		for _, m := range rec.Moves {
			if m.State == MovePending {
				return false
			}
		}
	}
	return true
}
