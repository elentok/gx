package triage

import "github.com/elentok/gx/tickets"

// Branch is what the skill does next.
type Branch string

const (
	BranchStart   Branch = "start"
	BranchVerify  Branch = "verify"
	BranchPlan    Branch = "plan"
	BranchApprove Branch = "approve"
	BranchWrite   Branch = "write"
	BranchDone    Branch = "done"
)

// Next detects the step from the record: no live record starts one, else rows
// with no verdict (verify), plan not complete (plan), plan saved and not
// approved (approve), approved and not done (write), done.
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
	case tickets.StepApprove:
		return BranchApprove
	}
	if len(rec.PendingVerdicts()) > 0 {
		return BranchVerify
	}
	return BranchPlan
}
