package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/nativerunner"
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
	if id := d.getenv(nativerunner.AgentSessionEnv); id != "" {
		on, err := d.readAutoAdvance(id)
		if err != nil {
			return err
		}
		if !on {
			fmt.Fprintln(out, "auto-advance is off")
			return &ExitError{Code: 1}
		}
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

// readServerAutoAdvance reads the switch live from the server. A session the
// server does not list is an error, so the caller stops instead of chaining.
func readServerAutoAdvance(dirs config.Dirs, sessionID string) (bool, error) {
	cl, err := serverClient(dirs)
	if err != nil {
		return false, err
	}
	sessions, err := cl.Sessions(context.Background())
	if err != nil {
		return false, fmt.Errorf("reading auto-advance for gx session %s: %w", sessionID, err)
	}
	for _, s := range sessions {
		if s.ID == sessionID {
			return s.AutoAdvance, nil
		}
	}
	return false, fmt.Errorf("gx session %s not found on the server", sessionID)
}
