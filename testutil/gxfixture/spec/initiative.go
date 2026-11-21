package spec

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/initiative"
)

// Initiative and Design names the Initiative scenarios seed.
const (
	// InitiativeSlug is the Initiative of the fresh, m1-implementing and m1-gated scenarios.
	InitiativeSlug = "ship-sync"
	// InitiativeDesign1 and InitiativeDesign2 are the Designs of Milestones 1 and 2.
	InitiativeDesign1 = "sync-core"
	InitiativeDesign2 = "sync-retry"
	// InitiativeBrokenSlug is the Initiative of the broken-link scenario.
	InitiativeBrokenSlug = "broken-trip"
	// InitiativeStrayDesign is the Design whose initiative: names no Initiative.
	InitiativeStrayDesign = "stray-design"
	// InitiativeArchivedDesign is the archived Design a Milestone links.
	InitiativeArchivedDesign = "old-design"
	// InitiativeMissingDesign is the Design a Milestone links that is not in the store.
	InitiativeMissingDesign = "gone-design"
)

// Lines that do not parse, one per structured section of the broken-link Initiative.
const (
	UnparsedMilestoneLine  = "- typed by hand, no pipes or goal"
	UnparsedAssumptionLine = "- A9 | typed by hand without its fields"
	UnparsedDeferredLine   = "- typed by hand | to: nowhere"
	UnparsedGateLine       = "remember to ask about this gate"
)

const initiativeMeta = "status: open\nkind: initiative\n"

const (
	implOpenMeta   = "status: open\n"
	implMergedMeta = "status: open\ncompleted_at: 2026-01-01T10:00:00Z\nmerged_at: 2026-01-01T11:00:00Z\n"
)

func designMeta(initiativeSlug, milestone string) string {
	return "status: open\nkind: map\ninitiative: " + initiativeSlug + "\nmilestone: " + milestone + "\n"
}

// must returns the rendered section text. The seeded values are constants, so
// a render error is a bug in the scenario.
func must(text string, err error) string {
	if err != nil {
		panic("initiative scenario: " + err.Error())
	}
	return text
}

// initiativeSections holds the typed content of an Initiative body; an empty
// section is left out.
type initiativeSections struct {
	prompt, destination string
	milestones          []initiative.Milestone
	assumptions         []initiative.Assumption
	deferred            []initiative.DeferredItem
	gate                []initiative.GateLine
	// raw lines are written verbatim after the rendered lines of a section.
	rawMilestones, rawAssumptions, rawDeferred, rawGate []string
}

func (s initiativeSections) body(title string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	add := func(heading, text string, raw []string) {
		lines := append(nonEmpty(text), raw...)
		if len(lines) == 0 {
			return
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", heading, strings.Join(lines, "\n"))
	}
	add(initiative.InitialPrompt, s.prompt, nil)
	add(initiative.Destination, s.destination, nil)

	var ms []initiative.Entry[initiative.Milestone]
	for _, m := range s.milestones {
		ms = append(ms, initiative.Parsed(m))
	}
	add(initiative.Milestones, must(initiative.RenderMilestones(ms)), s.rawMilestones)

	var as []initiative.Entry[initiative.Assumption]
	for _, a := range s.assumptions {
		as = append(as, initiative.Parsed(a))
	}
	add(initiative.Assumptions, must(initiative.RenderAssumptions(as)), s.rawAssumptions)

	var ds []initiative.Entry[initiative.DeferredItem]
	for _, d := range s.deferred {
		ds = append(ds, initiative.Parsed(d))
	}
	add(initiative.Deferred, must(initiative.RenderDeferredItems(ds)), s.rawDeferred)

	var gs []initiative.Entry[initiative.GateLine]
	for _, g := range s.gate {
		gs = append(gs, initiative.Parsed(g))
	}
	add(initiative.GateNotes, must(initiative.RenderGateNotes(gs)), s.rawGate)
	return strings.TrimSuffix(b.String(), "\n")
}

func nonEmpty(text string) []string {
	if text == "" {
		return nil
	}
	return []string{text}
}

func initiativeEpic(name string) epic { return epic{project, name, nil} }

func initiativeScenario(slug string, sections initiativeSections) scenario {
	addr := project + ":" + slug
	return scenario{
		Epics:    []epic{initiativeEpic(slug)},
		EpicMeta: map[string]string{addr: initiativeMeta},
		EpicBody: map[string]string{addr: sections.body(slug)},
	}
}

// addDesign seeds a Design map of the Initiative at the given step, and
// optionally its -impl epic (merged or not).
func addDesign(sc *scenario, design, milestone string, step, impl string) {
	addr := project + ":" + design
	sc.Epics = append(sc.Epics, initiativeEpic(design))
	sc.EpicMeta[addr] = designMeta(InitiativeSlug, milestone)
	if sc.DesignSteps == nil {
		sc.DesignSteps = map[string]string{}
	}
	sc.DesignSteps[addr] = step
	if impl == "" {
		return
	}
	implAddr := addr + tickets.ImplEpicSuffix
	sc.Epics = append(sc.Epics, epic{project, design + tickets.ImplEpicSuffix, []ticket{{"01", "build", servertest.TicketOpts{}}}})
	sc.EpicMeta[implAddr] = impl
}

// InitiativeFreshScenario is a new Initiative holding only its Initial prompt.
func InitiativeFreshScenario() scenario {
	return initiativeScenario(InitiativeSlug, initiativeSections{
		prompt: "Make sync reliable on flaky networks.",
	})
}

func initiativeMilestones() []initiative.Milestone {
	return []initiative.Milestone{
		{Slug: "core", Name: "Core sync", Goal: "Pushes and pulls finish on a clean network", Design: InitiativeDesign1},
		{Slug: "retry", Name: "Retry", Goal: "A dropped connection resumes where it stopped"},
		{Slug: "report", Name: "Report", Goal: "A failed sync tells the user why"},
	}
}

func initiativeAssumptions() []initiative.Assumption {
	return []initiative.Assumption{
		initiative.NewAssumption(1, "The remote accepts a resumed push", "core"),
		initiative.NewAssumption(2, "Users retry by hand today", "retry"),
	}
}

func initiativeSectionsBase() initiativeSections {
	return initiativeSections{
		prompt:      "Make sync reliable on flaky networks.",
		destination: "Sync finishes or says why it could not, on any network.",
		milestones:  initiativeMilestones(),
	}
}

// InitiativeM1ImplementingScenario is an active Initiative whose Milestone 1
// has a Design and an -impl epic that is not merged.
func InitiativeM1ImplementingScenario() scenario {
	s := initiativeSectionsBase()
	s.assumptions = initiativeAssumptions()
	s.deferred = []initiative.DeferredItem{
		{Item: "Proxy support", To: "retry", Why: "Needs the retry loop first", From: "core", State: initiative.DeferredOpen},
		{Item: "Bandwidth limit", To: initiative.Unplaced, Why: "No Milestone owns it yet", From: "core", State: initiative.DeferredOpen},
	}
	sc := initiativeScenario(InitiativeSlug, s)
	addDesign(&sc, InitiativeDesign1, "core", string(tickets.StepDone), implOpenMeta)
	return sc
}

// InitiativeM1GatedScenario is an Initiative whose Milestone 1 is gated, with
// a merged -impl epic, and whose Milestone 2 is being designed.
func InitiativeM1GatedScenario() scenario {
	s := initiativeSectionsBase()
	s.milestones[1].Design = InitiativeDesign2
	a := initiativeAssumptions()
	a[0].State, a[0].Evidence = initiative.StateConfirmed, "resumed push passed in sync-core"
	a[1].State, a[1].Evidence = initiative.StateBroken, "users give up after one failure"
	s.assumptions = a
	s.deferred = []initiative.DeferredItem{
		{Item: "Proxy support", To: "retry", Why: "Needs the retry loop first", From: "core", State: initiative.DeferredTaken},
		{Item: "Bandwidth limit", To: initiative.Unplaced, Why: "No Milestone owns it yet", From: "core", State: initiative.DeferredOpen},
	}
	s.gate = []initiative.GateLine{
		{Kind: initiative.GateHeading, Slug: "core"},
		{Kind: initiative.GateOutcome, Outcome: initiative.OutcomeContinue},
		{Kind: initiative.GateAssumption, N: 1, State: initiative.StateConfirmed, Evidence: "resumed push passed in sync-core"},
		{Kind: initiative.GateAssumption, N: 2, State: initiative.StateBroken, Evidence: "users give up after one failure"},
		{Kind: initiative.GateTaught, Text: "A resumed push needs the remote to keep partial data"},
		{Kind: initiative.GatePlacement, Text: "Proxy support goes to retry"},
	}
	sc := initiativeScenario(InitiativeSlug, s)
	addDesign(&sc, InitiativeDesign1, "core", string(tickets.StepDone), implMergedMeta)
	addDesign(&sc, InitiativeDesign2, "retry", string(tickets.StepChart), "")
	return sc
}

// InitiativeBrokenLinksScenario is an Initiative with every link problem the
// read verbs report: a Milestone whose Design is missing, a Design naming no
// Initiative, one hand-typed line in each structured section, and a Milestone
// whose Design is archived, which counts as existing.
func InitiativeBrokenLinksScenario() scenario {
	s := initiativeSections{
		prompt: "Check the broken link reports.",
		milestones: []initiative.Milestone{
			{Slug: "missing", Name: "Missing Design", Goal: "Its Design is gone", Design: InitiativeMissingDesign},
			{Slug: "archived", Name: "Archived Design", Goal: "Its Design is archived", Design: InitiativeArchivedDesign},
			{Slug: "plain", Name: "Plain", Goal: "Nothing linked"},
		},
		assumptions: []initiative.Assumption{initiative.NewAssumption(1, "A parsed line", "plain")},
		deferred: []initiative.DeferredItem{
			{Item: "A parsed item", To: initiative.Unplaced, Why: "Kept", From: "plain", State: initiative.DeferredOpen},
		},
		rawMilestones:  []string{UnparsedMilestoneLine},
		rawAssumptions: []string{UnparsedAssumptionLine},
		rawDeferred:    []string{UnparsedDeferredLine},
		rawGate:        []string{UnparsedGateLine},
	}
	sc := initiativeScenario(InitiativeBrokenSlug, s)
	sc.Epics = append(sc.Epics, initiativeEpic(InitiativeStrayDesign))
	sc.EpicMeta[project+":"+InitiativeStrayDesign] = designMeta("no-such-initiative", "plain")
	sc.DesignSteps = map[string]string{project + ":" + InitiativeStrayDesign: string(tickets.StepChart)}
	sc.Archived = []ArchivedEpic{{
		Project: project, Name: InitiativeArchivedDesign,
		Meta: designMeta(InitiativeBrokenSlug, "archived") + "archived_at: 2026-02-01T10:00:00Z\n",
	}}
	return sc
}
