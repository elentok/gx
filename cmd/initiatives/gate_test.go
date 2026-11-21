package initiatives

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/initiative"
)

const gateBody = "## Initial prompt\n\nidea\n\n" +
	"## Milestones\n\n" + unparsedMilestone + "\n- m1 | One | goal: g1 | design: d1\n- m2 | Two | goal: g2\n\n" +
	"## Assumptions\n\n" + unparsedAssumption + "\n" +
	"- A1 | first | tested by: m1 | state: unknown | evidence: -\n" +
	"- A2 | second | tested by: m2 | state: unknown | evidence: -\n\n" +
	"## Deferred\n\n" + unparsedDeferred + "\n" +
	"- open1 | to: m2 | why: later | from: d1 | state: open\n" +
	"- open2 | to: m2 | why: later | from: d1 | state: open\n" +
	"- taken1 | to: m2 | why: later | from: d1 | state: taken\n" +
	"- dropped1 | to: m2 | why: earlier | from: d1 | state: dropped\n\n" +
	"## Gate notes\n\nhand edited gate line\n"

const lastGateBody = "## Initial prompt\n\nidea\n\n## Milestones\n\n- m1 | One | goal: g1 | design: d1\n\n" +
	"## Deferred\n\n- open1 | to: m1 | why: later | from: d1 | state: open\n"

func gateSetup(t *testing.T, body string, merged bool) (repo, ticket string) {
	t.Helper()
	repo, project := setup(t)
	seedDesign(t, project, merged)
	return repo, seedInitiative(t, project, body)
}

func gateView(t *testing.T, ticket string) initiative.View {
	t.Helper()
	live, err := tickets.Load(filepath.Dir(filepath.Dir(ticket)))
	if err != nil {
		t.Fatal(err)
	}
	return initiative.Read(testEpic(readFile(t, ticket)), live, nil)
}

func gate(slug, outcome string, extra ...string) []string {
	return append([]string{"gate", "mine:init", slug, "--outcome", outcome}, extra...)
}

func TestGate_AppendsTheNoteAndAppliesItToAssumptionsAndDeferred(t *testing.T) {
	repo, ticket := gateSetup(t, gateBody, true)

	_, err := run(t, repo, gate("m1", "reshape",
		"--assumption", "A1=confirmed=it held",
		"--taught", "we learned x",
		"--placed", "open1 -> unplaced")...)
	if err != nil {
		t.Fatal(err)
	}
	b := parsedBody(t, ticket)
	want := []initiative.GateLine{
		{Kind: initiative.GateHeading, Slug: "m1"},
		{Kind: initiative.GateOutcome, Outcome: "reshape"},
		{Kind: initiative.GateAssumption, N: 1, State: "confirmed", Evidence: "it held"},
		{Kind: initiative.GateTaught, Text: "we learned x"},
		{Kind: initiative.GatePlacement, Text: "open1 -> unplaced"},
	}
	var got []initiative.GateLine
	for _, e := range b.GateNotes[1:] {
		got = append(got, e.Value)
	}
	if !slices.Equal(got, want) {
		t.Errorf("gate note = %+v, want %+v", got, want)
	}
	if a := b.Assumptions[1].Value; a.State != "confirmed" || a.Evidence != "it held" {
		t.Errorf("A1 = %+v", a)
	}
	if a := b.Assumptions[2].Value; a.State != "unknown" || a.Evidence != "-" {
		t.Errorf("A2 touched: %+v", a)
	}
	if d := deferredBy(t, ticket, "open1"); d.To != initiative.Unplaced || d.State != initiative.DeferredOpen {
		t.Errorf("open1 = %+v", d)
	}
	if d := deferredBy(t, ticket, "open2"); d.To != "m2" {
		t.Errorf("open2 touched: %+v", d)
	}
	if v := gateView(t, ticket); v.Milestones[0].State != initiative.MilestoneGated {
		t.Errorf("m1 state = %s", v.Milestones[0].State)
	}
}

func TestGate_KeepsUnparsedLinesInPlace(t *testing.T) {
	repo, ticket := gateSetup(t, gateBody, true)

	if _, err := run(t, repo, gate("m1", "continue", "--assumption", "A1=broken=no")...); err != nil {
		t.Fatal(err)
	}
	assertUnparsedKept(t, ticket)
	if g := parsedBody(t, ticket).GateNotes[0]; !g.Unparsed() || g.Raw != "hand edited gate line" {
		t.Errorf("unparsed gate line moved: %+v", g)
	}
}

func TestGate_RefusesAMilestoneWhoseImplEpicIsNotMerged(t *testing.T) {
	// sketch
	repo, ticket := gateSetup(t, gateBody, true)
	refuses(t, repo, ticket, "is sketch", gate("m2", "continue")...)

	// implementing
	repo, ticket = gateSetup(t, gateBody, false)
	refuses(t, repo, ticket, "is implementing", gate("m1", "continue")...)

	// designing
	repo, project := setup(t)
	seedDesign(t, project, false)
	if err := os.RemoveAll(filepath.Join(project, "d1-impl")); err != nil {
		t.Fatal(err)
	}
	ticket = seedInitiative(t, project, gateBody)
	refuses(t, repo, ticket, "is designing", gate("m1", "continue")...)
}

func TestGate_RefusesASecondGate(t *testing.T) {
	repo, ticket := gateSetup(t, gateBody, true)

	if _, err := run(t, repo, gate("m1", "continue")...); err != nil {
		t.Fatal(err)
	}
	refuses(t, repo, ticket, "already has a gate note", gate("m1", "continue")...)
}

func TestGate_DoneNeedsTheLastMilestone(t *testing.T) {
	repo, ticket := gateSetup(t, gateBody, true)
	refuses(t, repo, ticket, "not the last", gate("m1", "done")...)

	repo, ticket = gateSetup(t, lastGateBody, true)
	if _, err := run(t, repo, gate("m1", "done")...); err != nil {
		t.Fatal(err)
	}
}

func TestGate_StopDropsOpenItemsWithTheReasonAndLeavesTheRest(t *testing.T) {
	repo, ticket := gateSetup(t, gateBody, true)

	refuses(t, repo, ticket, "--reason", gate("m1", "stop")...)
	if _, err := run(t, repo, gate("m1", "stop", "--reason", "too costly")...); err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"open1", "open2"} {
		if d := deferredBy(t, ticket, item); d.State != initiative.DeferredDropped || d.Why != "too costly" {
			t.Errorf("%s = %+v", item, d)
		}
	}
	if d := deferredBy(t, ticket, "taken1"); d.State != initiative.DeferredTaken || d.Why != "later" {
		t.Errorf("taken1 = %+v", d)
	}
	if d := deferredBy(t, ticket, "dropped1"); d.Why != "earlier" {
		t.Errorf("dropped1 = %+v", d)
	}
	v := gateView(t, ticket)
	if v.State != initiative.StateStopped {
		t.Errorf("state = %s, want stopped", v.State)
	}
	if got := readFile(t, ticket); !strings.Contains(got, "status: open") {
		t.Errorf("Initiative closed: %s", got)
	}
}

func TestGate_DoneDropsOpenItemsAndFinishes(t *testing.T) {
	repo, ticket := gateSetup(t, lastGateBody, true)

	if _, err := run(t, repo, gate("m1", "done")...); err != nil {
		t.Fatal(err)
	}
	if d := deferredBy(t, ticket, "open1"); d.State != initiative.DeferredDropped || d.Why != "Initiative done" {
		t.Errorf("open1 = %+v", d)
	}
	if v := gateView(t, ticket); v.State != initiative.StateFinished {
		t.Errorf("state = %s, want finished", v.State)
	}
}

func TestGate_RefusesBadInputAndChangesNothing(t *testing.T) {
	repo, ticket := gateSetup(t, gateBody, true)

	refuses(t, repo, ticket, `unknown Milestone "nope"`, gate("nope", "continue")...)
	refuses(t, repo, ticket, `no Assumption "A9"`, gate("m1", "continue", "--assumption", "A9=confirmed=x")...)
	refuses(t, repo, ticket, "state", gate("m1", "continue", "--assumption", "A1=maybe=x")...)
	refuses(t, repo, ticket, "outcome", gate("m1", "bogus")...)
	refuses(t, repo, ticket, "must not contain", gate("m1", "continue", "--assumption", "A1=confirmed=a | b")...)
	refuses(t, repo, ticket, "must not contain", gate("m1", "continue", "--taught", "a | b")...)
	refuses(t, repo, ticket, "want A<n>", gate("m1", "continue", "--assumption", "A1")...)
	refuses(t, repo, ticket, `no deferred item "nope"`, gate("m1", "continue", "--placed", "nope -> m2")...)
	refuses(t, repo, ticket, `unknown Milestone "zz"`, gate("m1", "continue", "--placed", "open1 -> zz")...)
}
