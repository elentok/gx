package tickets

import "github.com/elentok/gx/tickets/schema"

// The design steps and flows live in schema to keep this package under its size
// limit; callers keep these names.
type (
	DesignStep = schema.DesignStep
	Flow       = schema.Flow
	FlowSpec   = schema.FlowSpec
)

const (
	StepChart          = schema.StepChart
	StepResolveTickets = schema.StepResolveTickets
	StepSpec           = schema.StepSpec
	StepReview         = schema.StepReview
	StepGrillFindings  = schema.StepGrillFindings
	StepCreateTickets  = schema.StepCreateTickets
	StepBranch         = schema.StepBranch
	StepDone           = schema.StepDone
	StepVerify         = schema.StepVerify
	StepPlan           = schema.StepPlan
	StepApprove        = schema.StepApprove
	StepWrite          = schema.StepWrite
	FlowDesign         = schema.FlowDesign
	FlowTriage         = schema.FlowTriage
)

var (
	DesignSteps     = schema.DesignSteps
	ParseDesignStep = schema.ParseDesignStep
	DesignStepNames = schema.DesignStepNames
	CheckDesignStep = schema.CheckDesignStep
	FlowSpecFor     = schema.FlowSpecFor
	CheckFlowStep   = schema.CheckFlowStep
)

// HasOpenTicket reports whether any of m's tickets is open or claimed.
func HasOpenTicket(m Epic) bool {
	for _, t := range m.Tickets {
		if t.Status == string(schema.StatusOpen) || t.Status == string(schema.StatusClaimed) {
			return true
		}
	}
	return false
}
