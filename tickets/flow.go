package tickets

import (
	"fmt"
	"slices"
)

// Flow is a guided sequence of steps a person walks with an agent.
type Flow string

const (
	FlowDesign Flow = "design"
	FlowTriage Flow = "triage"
)

// Triage step names. "done" is shared with Design (StepDone).
const (
	StepVerify  = "verify"
	StepPlan    = "plan"
	StepApprove = "approve"
	StepWrite   = "write"
)

// FlowSpec describes one Flow.
type FlowSpec struct {
	Steps  []string
	Labels map[string]string
	// Final steps end the flow.
	Final []string
	// NeedsPerson steps have no Continue and no countdown after them.
	NeedsPerson []string
	Skill       string
	// ContinueArg builds the skill argument from the flow's subject (a map
	// address for Design, a project for Triage).
	ContinueArg func(subject string) string
}

var flowSpecs = map[Flow]FlowSpec{
	FlowDesign: {
		Steps: designStepStrings(),
		Labels: map[string]string{
			string(StepChart): "Chart", string(StepResolveTickets): "Resolve tickets",
			string(StepSpec): "Spec", string(StepReview): "Review",
			string(StepGrillFindings): "Grill findings", string(StepCreateTickets): "Create tickets",
			string(StepBranch): "Branch", string(StepDone): "Done",
		},
		Final:       []string{string(StepBranch), string(StepDone)},
		Skill:       "gx-design",
		ContinueArg: func(mapAddr string) string { return mapAddr },
	},
	FlowTriage: {
		Steps: []string{StepVerify, StepPlan, StepApprove, StepWrite, string(StepDone)},
		Labels: map[string]string{
			StepVerify: "Verify", StepPlan: "Plan", StepApprove: "Approve",
			StepWrite: "Write", string(StepDone): "Done",
		},
		Final:       []string{string(StepDone)},
		NeedsPerson: []string{StepPlan, StepApprove},
		Skill:       "gx-triage-follow-ups",
		ContinueArg: func(project string) string { return "--project " + project },
	},
}

func designStepStrings() []string {
	return DesignStepNames()
}

// FlowSpecFor returns the spec of f, or an error for an unknown flow.
func FlowSpecFor(f Flow) (FlowSpec, error) {
	spec, ok := flowSpecs[f]
	if !ok {
		return FlowSpec{}, fmt.Errorf("unknown flow %q", f)
	}
	return spec, nil
}

// CheckFlowStep refuses a step that is not one of f's steps.
func CheckFlowStep(f Flow, step string) error {
	spec, err := FlowSpecFor(f)
	if err != nil {
		return err
	}
	if !slices.Contains(spec.Steps, step) {
		return fmt.Errorf("%s step %q; want one of %v", f, step, spec.Steps)
	}
	return nil
}

// IsFinal reports whether step ends the flow.
func (s FlowSpec) IsFinal(step string) bool { return slices.Contains(s.Final, step) }

// StepNeedsPerson reports whether step has no Continue.
func (s FlowSpec) StepNeedsPerson(step string) bool { return slices.Contains(s.NeedsPerson, step) }
