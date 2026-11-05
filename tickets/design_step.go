package tickets

import (
	"fmt"
	"slices"
	"strings"

	"github.com/elentok/gx/tickets/schema"
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

// HasOpenTicket reports whether any of m's tickets is open or claimed.
func HasOpenTicket(m Epic) bool {
	for _, t := range m.Tickets {
		if t.Status == string(schema.StatusOpen) || t.Status == string(schema.StatusClaimed) {
			return true
		}
	}
	return false
}
