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

// Final reports whether no run follows the record's step: no live record, the
// approve and done steps, and a write with no row left pending. Verify and plan
// are not final; plan's last save moves the step to approve before its run ends.
func Final(rec *Record) bool {
	if rec == nil || rec.State != StateLive {
		return true
	}
	switch rec.Step {
	case tickets.StepVerify, tickets.StepPlan:
		return false
	case tickets.StepWrite:
		for _, row := range rec.Rows {
			if row.Write == WritePending {
				return false
			}
		}
	}
	return true
}
