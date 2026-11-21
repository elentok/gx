package initiatives

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets/initiative"
)

const unparsedDestination = "hand edited destination line"

// m1 has a Design (d1, merged by seedDesign), m2 has a gate note, m3..m5 are
// sketches.
const reshapeBody = "## Initial prompt\n\nidea\n\n" +
	"## Destination\n\nold destination\n" + unparsedDestination + "\n\n" +
	"## Milestones\n\n" + unparsedMilestone + "\n" +
	"- m1 | One | goal: g1 | design: d1\n- m2 | Two | goal: g2\n- m3 | Three | goal: g3\n- m4 | Four | goal: g4\n- m5 | Five | goal: g5\n\n" +
	"## Assumptions\n\n" + unparsedAssumption + "\n" +
	"- A1 | first | tested by: m3 | state: unknown | evidence: -\n" +
	"- A2 | second | tested by: m5 | state: unknown | evidence: -\n\n" +
	"## Deferred\n\n" + unparsedDeferred + "\n" +
	"- open4 | to: m4 | why: later | from: d1 | state: open\n" +
	"- taken4 | to: m4 | why: later | from: d1 | state: taken\n" +
	"- dropped4 | to: m4 | why: later | from: d1 | state: dropped\n" +
	"- open5 | to: m5 | why: later | from: d1 | state: open\n\n" +
	"## Gate notes\n\n### m2\noutcome: continue\n"

func reshapeSetup(t *testing.T) (repo, ticket string) {
	t.Helper()
	repo, project := setup(t)
	seedDesign(t, project, true)
	return repo, seedInitiative(t, project, reshapeBody)
}

func slugs(t *testing.T, ticket string) []string {
	t.Helper()
	var out []string
	for _, e := range parsedBody(t, ticket).Milestones {
		if !e.Unparsed() {
			out = append(out, e.Value.Slug)
		}
	}
	return out
}

func deferredBy(t *testing.T, ticket, item string) initiative.DeferredItem {
	t.Helper()
	for _, e := range parsedBody(t, ticket).Deferred {
		if !e.Unparsed() && e.Value.Item == item {
			return e.Value
		}
	}
	t.Fatalf("no Deferred item %q", item)
	return initiative.DeferredItem{}
}

func assertUnparsedKept(t *testing.T, ticket string) {
	t.Helper()
	b := parsedBody(t, ticket)
	if !b.Milestones[0].Unparsed() || b.Milestones[0].Raw != unparsedMilestone {
		t.Errorf("unparsed Milestone line moved: %+v", b.Milestones[0])
	}
	if !b.Assumptions[0].Unparsed() || b.Assumptions[0].Raw != unparsedAssumption {
		t.Errorf("unparsed Assumption line moved: %+v", b.Assumptions[0])
	}
	if !b.Deferred[0].Unparsed() || b.Deferred[0].Raw != unparsedDeferred {
		t.Errorf("unparsed Deferred line moved: %+v", b.Deferred[0])
	}
}

func TestReshapeRemove_DeletesASketchAndPlacesItsOpenDeferred(t *testing.T) {
	repo, ticket := reshapeSetup(t)

	if _, err := run(t, repo, "milestone", "remove", "mine:init", "m4"); err != nil {
		t.Fatal(err)
	}
	if got := slugs(t, ticket); !slices.Equal(got, []string{"m1", "m2", "m3", "m5"}) {
		t.Errorf("Milestones = %v", got)
	}
	if got := deferredBy(t, ticket, "open4"); got.To != initiative.Unplaced || got.State != initiative.DeferredOpen {
		t.Errorf("open4 = %+v", got)
	}
	for _, item := range []string{"taken4", "dropped4"} {
		if got := deferredBy(t, ticket, item); got.To != "m4" {
			t.Errorf("%s = %+v, want to: m4 kept", item, got)
		}
	}
	if got := deferredBy(t, ticket, "open5"); got.To != "m5" {
		t.Errorf("open5 = %+v", got)
	}
	assertUnparsedKept(t, ticket)
}

func TestReshapeRemove_Refusals(t *testing.T) {
	repo, ticket := reshapeSetup(t)

	refuses(t, repo, ticket, "has a Design link", "milestone", "remove", "mine:init", "m1")
	refuses(t, repo, ticket, "has a gate note", "milestone", "remove", "mine:init", "m2")
	refuses(t, repo, ticket, "A1", "milestone", "remove", "mine:init", "m3")
	refuses(t, repo, ticket, "move them to another Milestone first", "milestone", "remove", "mine:init", "m3")
	refuses(t, repo, ticket, `unknown Milestone "nope"`, "milestone", "remove", "mine:init", "nope")
}

func TestReshapeRemove_AfterMovingTestedByAtAGate(t *testing.T) {
	repo, ticket := reshapeSetup(t)

	if _, err := run(t, repo, "assumption", "set", "mine:init", "A1", "--tested-by", "m4"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, repo, "milestone", "remove", "mine:init", "m3"); err != nil {
		t.Fatal(err)
	}
	if got := slugs(t, ticket); slices.Contains(got, "m3") {
		t.Errorf("Milestones = %v", got)
	}
	assertUnparsedKept(t, ticket)
}

func TestReshapeMove_ReordersASketchAfterTheCurrentMilestone(t *testing.T) {
	repo, ticket := reshapeSetup(t)

	if _, err := run(t, repo, "milestone", "move", "mine:init", "m5", "--to", "3"); err != nil {
		t.Fatal(err)
	}
	if got := slugs(t, ticket); !slices.Equal(got, []string{"m1", "m2", "m5", "m3", "m4"}) {
		t.Errorf("Milestones = %v", got)
	}
	assertUnparsedKept(t, ticket)
}

func TestReshapeMove_Refusals(t *testing.T) {
	repo, ticket := reshapeSetup(t)

	refuses(t, repo, ticket, "has a Design link", "milestone", "move", "mine:init", "m1", "--to", "4")
	refuses(t, repo, ticket, "has a gate note", "milestone", "move", "mine:init", "m2", "--to", "4")
	refuses(t, repo, ticket, "not after Milestone", "milestone", "move", "mine:init", "m5", "--to", "2")
	refuses(t, repo, ticket, "not after Milestone", "milestone", "move", "mine:init", "m5", "--to", "1")
	refuses(t, repo, ticket, "out of range", "milestone", "move", "mine:init", "m5", "--to", "9")
	refuses(t, repo, ticket, `unknown Milestone "nope"`, "milestone", "move", "mine:init", "nope", "--to", "4")
}

func TestReshapeInsert_PutsANewMilestoneAfterTheLockedOnes(t *testing.T) {
	repo, ticket := reshapeSetup(t)

	if _, err := run(t, repo, "milestone", "add", "mine:init", "m6", "--name", "Six", "--goal", "g6", "--at", "4"); err != nil {
		t.Fatal(err)
	}
	if got := slugs(t, ticket); !slices.Equal(got, []string{"m1", "m2", "m3", "m6", "m4", "m5"}) {
		t.Errorf("Milestones = %v", got)
	}
	assertUnparsedKept(t, ticket)
}

func TestReshapeInsert_Refusals(t *testing.T) {
	repo, ticket := reshapeSetup(t)
	add := []string{"milestone", "add", "mine:init", "m6", "--name", "Six", "--goal", "g6", "--at"}

	refuses(t, repo, ticket, "not after Milestone", append(slices.Clone(add), "1")...)
	refuses(t, repo, ticket, "not after Milestone", append(slices.Clone(add), "2")...)
	refuses(t, repo, ticket, "out of range", append(slices.Clone(add), "7")...)
}

func TestReshapeDestination_ReplacesTheFirstLineAndKeepsTheRest(t *testing.T) {
	repo, ticket := reshapeSetup(t)
	before := readFile(t, ticket)

	if _, err := run(t, repo, "destination", "mine:init", "new destination"); err != nil {
		t.Fatal(err)
	}
	after := readFile(t, ticket)
	want := strings.Replace(before, "old destination\n", "new destination\n", 1)
	if after != want {
		t.Errorf("file changed beyond the Destination line:\n%s", after)
	}
	if !strings.Contains(schemaSection(after, "Destination"), unparsedDestination) {
		t.Errorf("unparsed Destination line lost: %q", schemaSection(after, "Destination"))
	}
}

func TestReshapeDestination_RefusesBlankText(t *testing.T) {
	repo, ticket := reshapeSetup(t)

	refuses(t, repo, ticket, "empty", "destination", "mine:init", "   ")
	refuses(t, repo, ticket, "empty", "destination", "mine:init", "")
}
