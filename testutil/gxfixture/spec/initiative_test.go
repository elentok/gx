package spec

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/initiative"
	"github.com/elentok/gx/tickets/schema"
)

// loadInitiativeScenario writes the scenario's epics the way the seeder does
// and loads them back, so the tests read the same files the CLI reads.
func loadInitiativeScenario(t *testing.T, sc scenario) (live, archived []tickets.Epic) {
	t.Helper()
	dir := t.TempDir()
	write := func(parent, name, meta, body string) {
		epicDir := filepath.Join(parent, name)
		if err := os.MkdirAll(epicDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(epicDir, "ticket.md"), []byte("---\n"+meta+"---\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range sc.Epics {
		addr := e.Project + ":" + e.Name
		write(dir, e.Name, sc.EpicMeta[addr], sc.EpicBody[addr])
		live = append(live, tickets.LoadEpic(dir, e.Name))
	}
	archiveDir := filepath.Join(dir, ".archive")
	for _, a := range sc.Archived {
		write(archiveDir, a.Name, a.Meta, "")
		archived = append(archived, tickets.LoadEpic(archiveDir, a.Name))
	}
	return live, archived
}

func findLive(t *testing.T, live []tickets.Epic, name string) tickets.Epic {
	t.Helper()
	for _, e := range live {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no epic %q", name)
	return tickets.Epic{}
}

func milestoneStates(v initiative.View) []string {
	var out []string
	for _, m := range v.Milestones {
		out = append(out, m.State)
	}
	return out
}

func TestInitiativeScenario_FreshHoldsOnlyTheInitialPrompt(t *testing.T) {
	t.Parallel()
	live, archived := loadInitiativeScenario(t, InitiativeFreshScenario())
	if len(live) != 1 || !live[0].IsInitiative {
		t.Fatalf("epics = %+v, want one Initiative", live)
	}
	body := live[0].Body
	if schema.Section(body, initiative.InitialPrompt) == "" {
		t.Error("no Initial prompt")
	}
	for _, heading := range []string{initiative.Destination, initiative.Milestones, initiative.Assumptions, initiative.Deferred, initiative.GateNotes} {
		if got := schema.Section(body, heading); got != "" {
			t.Errorf("section %q = %q, want empty", heading, got)
		}
	}
	if v := initiative.Read(live[0], live, archived); v.Total != 0 || len(v.Unparsed) != 0 {
		t.Errorf("view = %+v, want no Milestones and no unparsed lines", v)
	}
}

func TestInitiativeScenario_M1ImplementingDerivesStatesFromTheStores(t *testing.T) {
	t.Parallel()
	live, archived := loadInitiativeScenario(t, InitiativeM1ImplementingScenario())
	init := findLive(t, live, InitiativeSlug)
	v := initiative.Read(init, live, archived)
	want := []string{initiative.MilestoneImplementing, initiative.MilestoneSketch, initiative.MilestoneSketch}
	if got := milestoneStates(v); !slices.Equal(got, want) {
		t.Errorf("Milestone states = %v, want %v", got, want)
	}
	if v.State != initiative.StateActive || len(v.Unparsed) != 0 || len(v.Problems) != 0 {
		t.Errorf("view = %+v, want active with no unparsed lines or problems", v)
	}
	if impl := findLive(t, live, InitiativeDesign1+tickets.ImplEpicSuffix); !impl.MergedAt.IsZero() {
		t.Errorf("impl epic merged_at = %v, want unset", impl.MergedAt)
	}
	body := initiative.ParseBody(init.Body)
	if len(body.Assumptions) != 2 {
		t.Fatalf("assumptions = %+v, want 2", body.Assumptions)
	}
	for _, a := range body.Assumptions {
		if a.Value.State != initiative.StateUnknown {
			t.Errorf("assumption A%d state = %q, want unknown", a.Value.N, a.Value.State)
		}
	}
	var unplaced int
	if len(body.Deferred) != 2 {
		t.Fatalf("deferred = %+v, want 2", body.Deferred)
	}
	for _, d := range body.Deferred {
		if d.Value.State != initiative.DeferredOpen {
			t.Errorf("deferred %q state = %q, want open", d.Value.Item, d.Value.State)
		}
		if d.Value.To == initiative.Unplaced {
			unplaced++
		}
	}
	if unplaced != 1 {
		t.Errorf("unplaced deferred items = %d, want 1", unplaced)
	}
	if len(body.GateNotes) != 0 {
		t.Errorf("gate notes = %+v, want none", body.GateNotes)
	}
}

func TestInitiativeScenario_M1GatedHasOneContinueGateNote(t *testing.T) {
	t.Parallel()
	live, archived := loadInitiativeScenario(t, InitiativeM1GatedScenario())
	init := findLive(t, live, InitiativeSlug)
	v := initiative.Read(init, live, archived)
	want := []string{initiative.MilestoneGated, initiative.MilestoneDesigning, initiative.MilestoneSketch}
	if got := milestoneStates(v); !slices.Equal(got, want) {
		t.Errorf("Milestone states = %v, want %v", got, want)
	}
	if v.Done != 1 || v.Current != "retry" || len(v.Unparsed) != 0 || len(v.Problems) != 0 {
		t.Errorf("view = %+v, want 1 gated, current retry, no unparsed lines or problems", v)
	}
	if impl := findLive(t, live, InitiativeDesign1+tickets.ImplEpicSuffix); impl.MergedAt.IsZero() {
		t.Error("Milestone 1 impl epic has no merged_at")
	}
	for _, e := range live {
		if e.Name == InitiativeDesign2+tickets.ImplEpicSuffix {
			t.Errorf("Milestone 2 has the impl epic %q", e.Name)
		}
	}
	body := initiative.ParseBody(init.Body)
	var states []string
	for _, a := range body.Assumptions {
		states = append(states, a.Value.State)
	}
	if !slices.Equal(states, []string{initiative.StateConfirmed, initiative.StateBroken}) {
		t.Errorf("assumption states = %v, want confirmed, broken", states)
	}
	if body.Assumptions[0].Value.Evidence == "-" {
		t.Error("the confirmed assumption has no evidence")
	}
	var items []string
	for _, d := range body.Deferred {
		items = append(items, d.Value.State)
	}
	if !slices.Equal(items, []string{initiative.DeferredTaken, initiative.DeferredOpen}) {
		t.Errorf("deferred states = %v, want taken, open", items)
	}
	counts := map[initiative.GateKind]int{}
	var outcome string
	for _, g := range body.GateNotes {
		counts[g.Value.Kind]++
		if g.Value.Kind == initiative.GateOutcome {
			outcome = g.Value.Outcome
		}
	}
	if counts[initiative.GateHeading] != 1 || outcome != initiative.OutcomeContinue || counts[initiative.GateAssumption] != 2 ||
		counts[initiative.GateTaught] != 1 || counts[initiative.GatePlacement] != 1 {
		t.Errorf("gate lines = %v outcome %q, want one continue note with 2 assumption lines, a taught and a placed line", counts, outcome)
	}
}

func TestInitiativeScenario_BrokenLinksReportEveryProblem(t *testing.T) {
	t.Parallel()
	live, archived := loadInitiativeScenario(t, InitiativeBrokenLinksScenario())
	init := findLive(t, live, InitiativeBrokenSlug)
	v := initiative.Read(init, live, archived)

	wantStates := []string{initiative.MilestoneBroken, initiative.MilestoneDesigning, initiative.MilestoneSketch}
	if got := milestoneStates(v); !slices.Equal(got, wantStates) {
		t.Errorf("Milestone states = %v, want %v (an archived Design exists)", got, wantStates)
	}
	if !v.Milestones[0].Broken || v.Milestones[1].Broken {
		t.Errorf("broken flags = %v, %v, want true, false", v.Milestones[0].Broken, v.Milestones[1].Broken)
	}
	var missing []string
	for _, p := range v.Problems {
		if p.Kind == initiative.ProblemMissingDesign {
			missing = append(missing, p.Design)
		}
	}
	if !slices.Equal(missing, []string{InitiativeMissingDesign}) {
		t.Errorf("missing Designs = %v, want only %s", missing, InitiativeMissingDesign)
	}

	var raw []initiative.UnparsedLine
	raw = append(raw, v.Unparsed...)
	want := []initiative.UnparsedLine{
		{Section: initiative.Milestones, Raw: UnparsedMilestoneLine},
		{Section: initiative.Assumptions, Raw: UnparsedAssumptionLine},
		{Section: initiative.Deferred, Raw: UnparsedDeferredLine},
		{Section: initiative.GateNotes, Raw: UnparsedGateLine},
	}
	if !slices.Equal(raw, want) {
		t.Errorf("unparsed lines = %+v, want %+v", raw, want)
	}

	stray := findLive(t, live, InitiativeStrayDesign)
	all := append(slices.Clone(live), archived...)
	link := initiative.BackLinkFor(stray, all)
	if link.Problem == nil || link.Problem.Kind != initiative.ProblemUnresolvedInitiative {
		t.Errorf("stray Design back-link = %+v, want an unresolved Initiative problem", link)
	}
	if got := initiative.UnresolvedInitiatives(all); len(got) != 1 || got[0].Design != InitiativeStrayDesign {
		t.Errorf("unresolved Initiatives = %+v, want only %s", got, InitiativeStrayDesign)
	}
}

func readScenario(t *testing.T, sc scenario, slug string) (initiative.View, initiative.Body, []tickets.Epic) {
	t.Helper()
	live, archived := loadInitiativeScenario(t, sc)
	init := findLive(t, live, slug)
	v := initiative.Read(init, live, archived)
	if len(v.Unparsed) != 0 || len(v.Problems) != 0 {
		t.Errorf("%s: unparsed %+v, problems %+v", slug, v.Unparsed, v.Problems)
	}
	return v, initiative.ParseBody(init.Body), live
}

func gateOutcomes(b initiative.Body) []string {
	var out []string
	for _, g := range b.GateNotes {
		if g.Value.Kind == initiative.GateOutcome {
			out = append(out, g.Value.Outcome)
		}
	}
	return out
}

func TestInitiativeScenario_StoppedIsGatedAtMilestone1(t *testing.T) {
	t.Parallel()
	v, body, _ := readScenario(t, InitiativeStoppedScenario(), InitiativeSlug)
	want := []string{initiative.MilestoneGated, initiative.MilestoneSketch, initiative.MilestoneSketch}
	if got := milestoneStates(v); v.State != initiative.StateStopped || v.StoppedAt != 1 || !slices.Equal(got, want) {
		t.Errorf("state %q stoppedAt %d milestones %v, want stopped at 1 with %v", v.State, v.StoppedAt, got, want)
	}
	if len(body.Assumptions) != 1 || body.Assumptions[0].Value.State != initiative.StateBroken {
		t.Errorf("assumptions = %+v, want one broken", body.Assumptions)
	}
	if len(body.Deferred) != 1 || body.Deferred[0].Value.State != initiative.DeferredDropped {
		t.Errorf("deferred = %+v, want one dropped", body.Deferred)
	}
	if got := gateOutcomes(body); !slices.Equal(got, []string{initiative.OutcomeStop}) {
		t.Errorf("gate outcomes = %v, want stop", got)
	}
	for _, g := range body.GateNotes {
		if g.Value.Kind == initiative.GatePlacement && g.Value.Text != "Proxy support dropped" {
			t.Errorf("placement = %q, want it to name the dropped item", g.Value.Text)
		}
	}
}

func TestInitiativeScenario_FinishedHasEmptyDeferredAndTwoGateNotes(t *testing.T) {
	t.Parallel()
	v, body, _ := readScenario(t, InitiativeFinishedScenario(), InitiativeSlug)
	if v.State != initiative.StateFinished || v.Done != 2 || v.Total != 2 || v.Current != "" {
		t.Errorf("view = %+v, want finished with 2 of 2 gated", v)
	}
	for _, a := range body.Assumptions {
		if a.Value.State != initiative.StateConfirmed {
			t.Errorf("assumption A%d state = %q, want confirmed", a.Value.N, a.Value.State)
		}
	}
	if len(body.Assumptions) != 2 || len(body.Deferred) != 0 {
		t.Errorf("assumptions %d, deferred %d, want 2 and 0", len(body.Assumptions), len(body.Deferred))
	}
	if got := gateOutcomes(body); !slices.Equal(got, []string{initiative.OutcomeContinue, initiative.OutcomeDone}) {
		t.Errorf("gate outcomes = %v, want continue, done", got)
	}
}

func TestInitiativeScenario_ListHasOneActiveAndOneStopped(t *testing.T) {
	t.Parallel()
	live, archived := loadInitiativeScenario(t, InitiativeListScenario())
	active := initiative.Read(findLive(t, live, InitiativeSlug), live, archived)
	if active.State != initiative.StateActive || active.Done != 1 || active.Total != 3 || active.Current != "retry" {
		t.Errorf("active = %+v, want active, 1 of 3 done, current retry", active)
	}
	stopped := initiative.Read(findLive(t, live, InitiativeStoppedSlug), live, archived)
	if stopped.State != initiative.StateStopped || stopped.StoppedAt != 1 {
		t.Errorf("stopped = %+v, want stopped at Milestone 1", stopped)
	}
	var initiatives int
	for _, e := range live {
		if e.IsInitiative {
			initiatives++
		}
	}
	if initiatives != 2 {
		t.Errorf("Initiatives = %d, want 2", initiatives)
	}
}

func TestInitiativeScenario_EpicsCardsSeedTwoBackLinksAndOnePlainDesign(t *testing.T) {
	t.Parallel()
	_, _, live := readScenario(t, InitiativeEpicsCardsScenario(), InitiativeSlug)
	for name, want := range map[string]int{InitiativeDesign1: 1, InitiativeDesign1 + tickets.ImplEpicSuffix: 1, InitiativeDesign2: 2} {
		link := initiative.BackLinkFor(findLive(t, live, name), live)
		if link.Milestone != want || link.Initiative != InitiativeSlug || link.Problem != nil {
			t.Errorf("%s back-link = %+v, want Milestone %d", name, link, want)
		}
	}
	if link := initiative.BackLinkFor(findLive(t, live, InitiativePlainDesign), live); link != (initiative.BackLink{}) {
		t.Errorf("plain Design back-link = %+v, want none", link)
	}
	for _, e := range live {
		if e.Name == InitiativeDesign2+tickets.ImplEpicSuffix {
			t.Errorf("Design card has the impl epic %q", e.Name)
		}
	}
	if !findLive(t, live, InitiativeSlug).IsInitiative {
		t.Error("the Initiative is not an Initiative")
	}
}

func TestInitiativeScenario_GateReadyMilestone1IsMerged(t *testing.T) {
	t.Parallel()
	v, body, _ := readScenario(t, InitiativeGateReadyScenario(), InitiativeSlug)
	if v.Milestones[0].State != initiative.MilestoneMerged || len(body.GateNotes) != 0 {
		t.Errorf("Milestone 1 state %q, gate notes %d, want merged and none", v.Milestones[0].State, len(body.GateNotes))
	}
}

func TestInitiativeScenario_ResumeContinuesToAnUndesignedMilestone(t *testing.T) {
	t.Parallel()
	v, body, _ := readScenario(t, InitiativeResumeScenario(), InitiativeSlug)
	want := []string{initiative.MilestoneGated, initiative.MilestoneSketch, initiative.MilestoneSketch}
	if got := milestoneStates(v); !slices.Equal(got, want) || v.State != initiative.StateActive || v.Milestones[1].Design != "" {
		t.Errorf("states %v active %q design %q, want %v, active, no Design", got, v.State, v.Milestones[1].Design, want)
	}
	if got := gateOutcomes(body); !slices.Equal(got, []string{initiative.OutcomeContinue}) {
		t.Errorf("gate outcomes = %v, want continue", got)
	}
}

// A seeded valid line that stops parsing back would show up as an unparsed
// line, so every scenario but the broken-link one has none.
func TestInitiativeScenario_SeededValidLinesParseBack(t *testing.T) {
	t.Parallel()
	for name, sc := range map[string]scenario{
		"fresh":        InitiativeFreshScenario(),
		"implementing": InitiativeM1ImplementingScenario(),
		"gated":        InitiativeM1GatedScenario(),
	} {
		live, archived := loadInitiativeScenario(t, sc)
		for _, e := range live {
			if e.IsInitiative {
				if v := initiative.Read(e, live, archived); len(v.Unparsed) != 0 {
					t.Errorf("%s: unparsed lines %+v", name, v.Unparsed)
				}
			}
		}
	}
	live, archived := loadInitiativeScenario(t, InitiativeBrokenLinksScenario())
	body := initiative.ParseBody(findLive(t, live, InitiativeBrokenSlug).Body)
	if len(body.Milestones) != 4 || len(body.Assumptions) != 2 || len(body.Deferred) != 2 || len(body.GateNotes) != 1 {
		t.Errorf("entries = %d, %d, %d, %d, want 4, 2, 2, 1", len(body.Milestones), len(body.Assumptions), len(body.Deferred), len(body.GateNotes))
	}
	if v := initiative.Read(findLive(t, live, InitiativeBrokenSlug), live, archived); len(v.Unparsed) != 4 {
		t.Errorf("unparsed lines = %+v, want exactly the 4 planted ones", v.Unparsed)
	}
}
