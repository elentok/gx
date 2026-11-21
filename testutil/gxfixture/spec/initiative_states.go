package spec

import (
	"maps"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/initiative"
)

// Names the Initiative page, list and Epics page scenarios seed besides the
// ones in initiative.go.
const (
	// InitiativeStoppedSlug and InitiativeStoppedDesign are the Initiative that
	// the list scenario stops at Milestone 1, and its Design.
	InitiativeStoppedSlug   = "stopped-sync"
	InitiativeStoppedDesign = "stopped-core"
	// InitiativePlainDesign is the Design with no initiative: on the Epics page.
	InitiativePlainDesign = "plain-design"
)

func gateNote(slug, outcome string, rest ...initiative.GateLine) []initiative.GateLine {
	lines := []initiative.GateLine{
		{Kind: initiative.GateHeading, Slug: slug},
		{Kind: initiative.GateOutcome, Outcome: outcome},
	}
	return append(lines, rest...)
}

// stoppedInitiative is an Initiative stopped at the gate of Milestone 1: its
// Design is done and merged, two sketches follow, one Assumption broke and one
// Deferred item was dropped.
func stoppedInitiative(slug, design string) scenario {
	s := initiativeSectionsBase()
	s.milestones[0].Design = design
	s.assumptions = []initiative.Assumption{initiative.NewAssumption(1, "The remote accepts a resumed push", "core")}
	s.assumptions[0].State = initiative.StateBroken
	s.assumptions[0].Evidence = "the remote drops partial data"
	s.deferred = []initiative.DeferredItem{
		{Item: "Proxy support", To: initiative.Unplaced, Why: "Not worth it without resume", From: "core", State: initiative.DeferredDropped},
	}
	s.gate = gateNote("core", initiative.OutcomeStop,
		initiative.GateLine{Kind: initiative.GateAssumption, N: 1, State: initiative.StateBroken, Evidence: "the remote drops partial data"},
		initiative.GateLine{Kind: initiative.GateTaught, Text: "The remote cannot resume a push"},
		initiative.GateLine{Kind: initiative.GatePlacement, Text: "Proxy support dropped"},
	)
	sc := initiativeScenario(slug, s)
	addDesignOf(&sc, slug, design, "core", string(tickets.StepDone), implMergedMeta)
	return sc
}

// InitiativeStoppedScenario is an Initiative stopped at the gate of Milestone 1.
func InitiativeStoppedScenario() scenario {
	return stoppedInitiative(InitiativeSlug, InitiativeDesign1)
}

// InitiativeFinishedScenario is an Initiative with both Milestones gated, the
// last with outcome done, and no Deferred items.
func InitiativeFinishedScenario() scenario {
	s := initiativeSectionsBase()
	s.milestones = s.milestones[:2]
	s.milestones[1].Design = InitiativeDesign2
	a := initiativeAssumptions()
	a[0].State, a[0].Evidence = initiative.StateConfirmed, "resumed push passed in sync-core"
	a[1].State, a[1].Evidence = initiative.StateConfirmed, "retry loop resumed in sync-retry"
	s.assumptions = a
	s.gate = append(
		gateNote("core", initiative.OutcomeContinue,
			initiative.GateLine{Kind: initiative.GateAssumption, N: 1, State: initiative.StateConfirmed, Evidence: "resumed push passed in sync-core"},
			initiative.GateLine{Kind: initiative.GateTaught, Text: "Resumed pushes work"},
			initiative.GateLine{Kind: initiative.GatePlacement, Text: "none"}),
		gateNote("retry", initiative.OutcomeDone,
			initiative.GateLine{Kind: initiative.GateAssumption, N: 2, State: initiative.StateConfirmed, Evidence: "retry loop resumed in sync-retry"},
			initiative.GateLine{Kind: initiative.GateTaught, Text: "Retries resume after a drop"},
			initiative.GateLine{Kind: initiative.GatePlacement, Text: "none"})...,
	)
	sc := initiativeScenario(InitiativeSlug, s)
	addDesign(&sc, InitiativeDesign1, "core", string(tickets.StepDone), implMergedMeta)
	addDesign(&sc, InitiativeDesign2, "retry", string(tickets.StepDone), implMergedMeta)
	return sc
}

// combine puts the epics of two scenarios side by side.
func combine(a, b scenario) scenario {
	a.Epics = append(a.Epics, b.Epics...)
	maps.Copy(a.EpicMeta, b.EpicMeta)
	maps.Copy(a.EpicBody, b.EpicBody)
	if a.DesignSteps == nil {
		a.DesignSteps = map[string]string{}
	}
	maps.Copy(a.DesignSteps, b.DesignSteps)
	return a
}

// InitiativeListScenario is one active Initiative (1 of 3 done) and one
// stopped at the gate of its Milestone 1. Nothing in it implies a Start button.
func InitiativeListScenario() scenario {
	return combine(InitiativeM1GatedScenario(), stoppedInitiative(InitiativeStoppedSlug, InitiativeStoppedDesign))
}

// InitiativeEpicsCardsScenario is an Initiative with an Implementation card
// (Design plus -impl epic), a Design card (no -impl epic) and a plain Design.
func InitiativeEpicsCardsScenario() scenario {
	s := initiativeSectionsBase()
	s.milestones[1].Design = InitiativeDesign2
	sc := initiativeScenario(InitiativeSlug, s)
	addDesign(&sc, InitiativeDesign1, "core", string(tickets.StepDone), implOpenMeta)
	addDesign(&sc, InitiativeDesign2, "retry", string(tickets.StepChart), "")
	plain := project + ":" + InitiativePlainDesign
	sc.Epics = append(sc.Epics, initiativeEpic(InitiativePlainDesign))
	sc.EpicMeta[plain] = "status: open\nkind: map\n"
	sc.DesignSteps[plain] = string(tickets.StepChart)
	return sc
}

// InitiativeGateReadyScenario is an Initiative whose Milestone 1 impl epic is
// merged and which has no gate note, so the Milestone is merged and ready.
func InitiativeGateReadyScenario() scenario {
	sc := initiativeScenario(InitiativeSlug, initiativeSectionsBase())
	addDesign(&sc, InitiativeDesign1, "core", string(tickets.StepDone), implMergedMeta)
	return sc
}

// InitiativeResumeScenario is an Initiative whose Milestone 1 is gated with
// outcome continue and whose next Milestone has no Design yet.
func InitiativeResumeScenario() scenario {
	s := initiativeSectionsBase()
	s.gate = gateNote("core", initiative.OutcomeContinue)
	sc := initiativeScenario(InitiativeSlug, s)
	addDesign(&sc, InitiativeDesign1, "core", string(tickets.StepDone), implMergedMeta)
	return sc
}
