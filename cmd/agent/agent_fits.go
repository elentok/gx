package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/tickets"
)

// stepGrowth is the padded median context growth of each design step. The
// grill-findings through branch steps run as one block, so they share a value.
// The Triage steps verify and write are estimates, not measurements: verify
// reads each follow-up and its code, so it is sized like the design Review
// step doubled; write drafts the resulting tickets, sized like Resolve
// tickets. Replace them with measured medians once Triage runs have data.
// "done" is shared by both flows.
var stepGrowth = map[string]int{
	string(tickets.StepSpec):           25_000,
	string(tickets.StepReview):         20_000,
	string(tickets.StepResolveTickets): 50_000,
	string(tickets.StepChart):          60_000,
	string(tickets.StepGrillFindings):  80_000,
	string(tickets.StepCreateTickets):  80_000,
	string(tickets.StepBranch):         80_000,
	tickets.StepVerify:                 40_000,
	tickets.StepWrite:                  50_000,
	string(tickets.StepDone):           0,
}

// checkFitsStep refuses a step that is neither a Design nor a Triage step.
func checkFitsStep(step string) error {
	if _, ok := stepGrowth[step]; ok {
		return nil
	}
	valid := make([]string, 0, len(stepGrowth))
	for _, f := range []tickets.Flow{tickets.FlowDesign, tickets.FlowTriage} {
		spec, _ := tickets.FlowSpecFor(f)
		for _, s := range spec.Steps {
			if _, ok := stepGrowth[s]; ok && !slices.Contains(valid, s) {
				valid = append(valid, s)
			}
		}
	}
	return fmt.Errorf("step %q; want one of %s", step, strings.Join(valid, ", "))
}

// runAgentFits prints the used tokens, the step's growth and the ceiling, and
// returns an ExitError{1} when the step would not stay under the ceiling.
// Any other error means the caller should treat the answer as "stop".
func runAgentFits(d Deps, out io.Writer, agentKind, session, cwd, stepName string) error {
	if err := checkFitsStep(stepName); err != nil {
		return err
	}
	if id := d.Getenv(holder.AgentSessionEnv); id != "" {
		on, err := d.ReadAutoAdvance(id)
		if err != nil {
			return err
		}
		if !on {
			fmt.Fprintln(out, "auto-advance is off")
			return &cmdbase.ExitError{Code: 1}
		}
	}
	used, err := runAgentContextWindow(d, agentKind, session, cwd)
	if errors.Is(err, errCodexNoUsage) {
		// A fresh Codex session has made no model call yet: it has used nothing.
		used, err = 0, nil
	}
	if err != nil {
		return err
	}
	growth := stepGrowth[stepName]
	ceiling := agent.SmartZoneCeiling
	fmt.Fprintf(out, "used=%d growth=%d ceiling=%d\n", used, growth, ceiling)
	// A session that has used nothing always fits: stopping cannot save any context.
	if stepName != string(tickets.StepDone) && used > 0 && used+growth >= ceiling {
		return &cmdbase.ExitError{Code: 1}
	}
	return nil
}

// readServerAutoAdvance reads the switch live from the server. A session the
// server does not list is an error, so the caller stops instead of chaining.
func readServerAutoAdvance(dirs config.Dirs, sessionID string) (bool, error) {
	s, err := findServerSession(dirs, sessionID)
	if err != nil {
		return false, err
	}
	return s.AutoAdvance, nil
}

// readServerHarness reads a gx session's harness name from the server.
func readServerHarness(dirs config.Dirs, sessionID string) (string, error) {
	s, err := findServerSession(dirs, sessionID)
	if err != nil {
		return "", err
	}
	return s.Harness, nil
}

func findServerSession(dirs config.Dirs, sessionID string) (wire.SessionInfo, error) {
	cl, err := cmdbase.ServerClient(dirs)
	if err != nil {
		return wire.SessionInfo{}, err
	}
	sessions, err := cl.Sessions.List(context.Background())
	if err != nil {
		return wire.SessionInfo{}, fmt.Errorf("reading gx session %s: %w", sessionID, err)
	}
	for _, s := range sessions {
		if s.ID == sessionID {
			return s, nil
		}
	}
	return wire.SessionInfo{}, fmt.Errorf("gx session %s not found on the server", sessionID)
}
