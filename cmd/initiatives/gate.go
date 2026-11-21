package initiatives

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/tickets/initiative"
)

// placedSep splits a --placed value into the item and its new placement. The
// same text is stored in the gate note.
const placedSep = " -> "

func newGate(d Deps) *cobra.Command {
	return storeCmd(d, "gate <project:initiative> <milestone>", "record the Gate of a merged Milestone", func(c *cobra.Command) {
		c.Flags().String("outcome", "", "continue, reshape, stop or done (required)")
		_ = c.MarkFlagRequired("outcome")
		c.Flags().StringArray("assumption", nil, "A<n>=<state>=<evidence>, once per Assumption")
		c.Flags().StringArray("taught", nil, "what the Milestone taught, once per item")
		c.Flags().StringArray("placed", nil, "<deferred item>"+placedSep+"<Milestone slug or unplaced>, once per item")
		c.Flags().String("reason", "", "why the Initiative stops (required for stop)")
	}, func(c *cobra.Command, args []string) (storeEdit, error) {
		outcome, _ := c.Flags().GetString("outcome")
		assumptions, _ := c.Flags().GetStringArray("assumption")
		taught, _ := c.Flags().GetStringArray("taught")
		placed, _ := c.Flags().GetStringArray("placed")
		reason, _ := c.Flags().GetString("reason")
		if outcome == initiative.OutcomeStop && reason == "" {
			return nil, fmt.Errorf("--reason is required for stop")
		}
		return func(b *initiative.Body, v initiative.View, _ epicSet) (string, error) {
			return applyGate(b, v, gateCall{args[1], outcome, assumptions, taught, placed, reason})
		}, nil
	})
}

type gateCall struct {
	slug, outcome               string
	assumptions, taught, placed []string
	reason                      string
}

func applyGate(b *initiative.Body, v initiative.View, g gateCall) (string, error) {
	if err := checkMilestone(v, g.slug); err != nil {
		return "", err
	}
	if err := checkGateable(v, g); err != nil {
		return "", err
	}
	lines := []initiative.GateLine{
		{Kind: initiative.GateHeading, Slug: g.slug},
		{Kind: initiative.GateOutcome, Outcome: g.outcome},
	}
	for _, s := range g.assumptions {
		line, err := applyGateAssumption(b, s)
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	for _, t := range g.taught {
		lines = append(lines, initiative.GateLine{Kind: initiative.GateTaught, Text: t})
	}
	if g.outcome == initiative.OutcomeStop {
		lines = append(lines, initiative.GateLine{Kind: initiative.GateTaught, Text: "stop reason: " + g.reason})
	}
	for _, p := range g.placed {
		if err := applyGatePlacement(b, v, p); err != nil {
			return "", err
		}
		lines = append(lines, initiative.GateLine{Kind: initiative.GatePlacement, Text: p})
	}
	switch g.outcome {
	case initiative.OutcomeStop:
		dropOpen(b, g.reason)
	case initiative.OutcomeDone:
		dropOpen(b, "Initiative done")
	}
	for _, l := range lines {
		b.GateNotes = append(b.GateNotes, initiative.Parsed(l))
	}
	return "### " + g.slug, nil
}

// checkGateable refuses unless the Milestone's impl epic is merged and it has
// no gate note yet, and refuses done on any Milestone but the last.
func checkGateable(v initiative.View, g gateCall) error {
	var m initiative.MilestoneView
	for _, mv := range v.Milestones {
		if mv.Slug == g.slug {
			m = mv
		}
	}
	switch m.State {
	case initiative.MilestoneMerged:
	case initiative.MilestoneGated:
		return fmt.Errorf("Milestone %q already has a gate note", g.slug)
	default:
		return fmt.Errorf("Milestone %q is %s: a Gate needs its impl epic merged", g.slug, m.State)
	}
	if g.outcome == initiative.OutcomeDone && v.Milestones[len(v.Milestones)-1].Slug != g.slug {
		return fmt.Errorf("outcome done needs the last Milestone, and %q is not the last", g.slug)
	}
	return initiative.ValidateOutcome(g.outcome)
}

func applyGateAssumption(b *initiative.Body, spec string) (initiative.GateLine, error) {
	parts := strings.SplitN(spec, "=", 3)
	if len(parts) != 3 {
		return initiative.GateLine{}, fmt.Errorf("--assumption %q: want A<n>=<state>=<evidence>", spec)
	}
	i, err := initiative.FindAssumption(b.Assumptions, parts[0])
	if err != nil {
		return initiative.GateLine{}, err
	}
	a := b.Assumptions[i].Value
	a.State, a.Evidence = parts[1], parts[2]
	if _, err := initiative.RenderAssumption(a); err != nil {
		return initiative.GateLine{}, err
	}
	b.Assumptions[i] = initiative.Parsed(a)
	return initiative.GateLine{Kind: initiative.GateAssumption, N: a.N, State: a.State, Evidence: a.Evidence}, nil
}

func applyGatePlacement(b *initiative.Body, v initiative.View, spec string) error {
	item, to, ok := strings.Cut(spec, placedSep)
	if !ok {
		return fmt.Errorf("--placed %q: want <item>%s<Milestone slug or unplaced>", spec, placedSep)
	}
	if err := checkPlacement(v, to); err != nil {
		return err
	}
	idx, err := initiative.FindDeferred(b.Deferred, item)
	if err != nil {
		return err
	}
	d := b.Deferred[idx].Value
	d.To = to
	if _, err := initiative.RenderDeferred(d); err != nil {
		return err
	}
	b.Deferred[idx] = initiative.Parsed(d)
	return nil
}

func dropOpen(b *initiative.Body, reason string) {
	for i, e := range b.Deferred {
		if e.Unparsed() || e.Value.State != initiative.DeferredOpen {
			continue
		}
		d := e.Value
		d.State, d.Why = initiative.DeferredDropped, reason
		b.Deferred[i] = initiative.Parsed(d)
	}
}
