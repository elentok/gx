package schema

import (
	"fmt"
	"slices"
	"strings"
)

// DesignStep is a value of a map epic's design_step. The values are also the
// Progress step wire values the server sends.
type DesignStep string

const (
	StepChart          DesignStep = "chart"
	StepResolveTickets DesignStep = "resolve-tickets"
	StepSpec           DesignStep = "spec"
	StepReview         DesignStep = "review"
	StepGrillFindings  DesignStep = "grill-findings"
	StepCreateTickets  DesignStep = "create-tickets"
	StepBranch         DesignStep = "branch"
	StepDone           DesignStep = "done"
)

// DesignSteps lists every DesignStep, in order.
var DesignSteps = []DesignStep{
	StepChart, StepResolveTickets, StepSpec, StepReview,
	StepGrillFindings, StepCreateTickets, StepBranch, StepDone,
}

// ParseDesignStep returns the DesignStep named by s, or false when s is not one.
func ParseDesignStep(s string) (DesignStep, bool) {
	step := DesignStep(s)
	return step, slices.Contains(DesignSteps, step)
}

// DesignStepNames is DesignSteps as strings, for messages and flag help.
func DesignStepNames() []string {
	names := make([]string, len(DesignSteps))
	for i, s := range DesignSteps {
		names[i] = string(s)
	}
	return names
}

// CheckDesignStep refuses a name that is not one of DesignSteps.
func CheckDesignStep(step string) error {
	if _, ok := ParseDesignStep(step); !ok {
		return fmt.Errorf("design step %q; want one of %s", step, strings.Join(DesignStepNames(), ", "))
	}
	return nil
}

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
		Steps: DesignStepNames(),
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
