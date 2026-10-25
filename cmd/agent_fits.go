package cmd

import (
	"fmt"
	"io"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// stepGrowth is the padded median context growth of each design step. The
// grill-findings through branch steps run as one block, so they share a value.
var stepGrowth = map[tickets.DesignStep]int{
	tickets.StepSpec:           25_000,
	tickets.StepReview:         20_000,
	tickets.StepResolveTickets: 50_000,
	tickets.StepChart:          60_000,
	tickets.StepGrillFindings:  150_000,
	tickets.StepCreateTickets:  150_000,
	tickets.StepBranch:         150_000,
	tickets.StepDone:           0,
}

// runAgentFits prints the used tokens, the step's growth and the ceiling, and
// returns an ExitError{1} when the step would not stay under the ceiling.
// Any other error means the caller should treat the answer as "stop".
func runAgentFits(d deps, out io.Writer, agent, session, cwd, stepName string) error {
	step, ok := tickets.ParseDesignStep(stepName)
	if !ok {
		return tickets.CheckDesignStep(stepName)
	}
	used, err := runAgentContextWindow(d, agent, session, cwd)
	if err != nil {
		return err
	}
	growth := stepGrowth[step]
	ceiling := ralphloop.SmartZoneCeiling
	fmt.Fprintf(out, "used=%d growth=%d ceiling=%d\n", used, growth, ceiling)
	if step != tickets.StepDone && used+growth >= ceiling {
		return &ExitError{Code: 1}
	}
	return nil
}
