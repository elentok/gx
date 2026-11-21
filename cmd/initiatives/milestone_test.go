package initiatives

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/initiative"
)

const unparsedMilestone = "- hand edited milestone"

const milestoneBody = "## Initial prompt\n\nidea\n\n" +
	"## Milestones\n\n" + unparsedMilestone + "\n- m1 | One | goal: g1 | design: d1\n- m2 | Two | goal: g2\n- m3 | Three | goal: g3\n\n" +
	"## Assumptions\n\n" + unparsedAssumption + "\n" +
	"- A1 | first | tested by: m2 | state: unknown | evidence: -\n" +
	"- A2 | second | tested by: m3 | state: unknown | evidence: -\n\n" +
	"## Deferred\n\n" + unparsedDeferred + "\n" +
	"- one | to: m2 | why: later | from: d1 | state: open\n" +
	"- two | to: m3 | why: later | from: d1 | state: open\n\n" +
	"## Gate notes\n\n### m3\noutcome: continue\n"

func testEpic(file string) tickets.Epic {
	return tickets.Epic{Name: "init", Body: strings.SplitN(file, "---\n", 3)[2]}
}

func milestoneBySlug(t *testing.T, ticket, slug string) initiative.Milestone {
	t.Helper()
	for _, e := range parsedBody(t, ticket).Milestones {
		if !e.Unparsed() && e.Value.Slug == slug {
			return e.Value
		}
	}
	t.Fatalf("no Milestone %q", slug)
	return initiative.Milestone{}
}

func TestMilestoneAdd_AppendsASketchAndParsesBack(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, milestoneBody)

	out, err := run(t, repo, "milestone", "add", "mine:init", "m4", "--name", "Four", "--goal", "g4")
	if err != nil || out != "- m4 | Four | goal: g4\n" {
		t.Fatalf("out = %q err = %v", out, err)
	}
	es := parsedBody(t, ticket).Milestones
	last := es[len(es)-1]
	if last.Unparsed() || last.Value != (initiative.Milestone{Slug: "m4", Name: "Four", Goal: "g4"}) {
		t.Errorf("last = %+v", last)
	}
	if !es[0].Unparsed() || es[0].Raw != unparsedMilestone {
		t.Errorf("unparsed line moved: %+v", es[0])
	}
}

func TestMilestoneAdd_RefusesClashesAndLeavesTheFileAlone(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, milestoneBody)
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "init-live"))
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "init-live2-design"))
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "init-live3-impl"))
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, ".archive", "init-old"))

	refuses(t, repo, ticket, `Milestone "m2" already exists`, "milestone", "add", "mine:init", "m2", "--name", "N", "--goal", "g")
	refuses(t, repo, ticket, `epic "init-live" already exists`, "milestone", "add", "mine:init", "live", "--name", "N", "--goal", "g")
	refuses(t, repo, ticket, `epic "init-live2-design" already exists`, "milestone", "add", "mine:init", "live2", "--name", "N", "--goal", "g")
	refuses(t, repo, ticket, `epic "init-live3-impl" already exists`, "milestone", "add", "mine:init", "live3", "--name", "N", "--goal", "g")
	refuses(t, repo, ticket, `epic "init-old" already exists`, "milestone", "add", "mine:init", "old", "--name", "N", "--goal", "g")
	refuses(t, repo, ticket, "not a valid slug", "milestone", "add", "mine:init", "Bad Slug", "--name", "N", "--goal", "g")
}

func TestMilestoneSetDesign_LinksOnlyTheNamedMilestone(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, milestoneBody)
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "d2"))
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, ".archive", "dold"))

	if _, err := run(t, repo, "milestone", "set", "mine:init", "m2", "--design", "d2"); err != nil {
		t.Fatal(err)
	}
	if got := milestoneBySlug(t, ticket, "m2").Design; got != "d2" {
		t.Errorf("m2 design = %q", got)
	}
	if got := milestoneBySlug(t, ticket, "m1").Design; got != "d1" {
		t.Errorf("m1 design = %q", got)
	}
	if _, err := run(t, repo, "milestone", "set", "mine:init", "m2", "--design", "dold"); err != nil {
		t.Errorf("archived Design: %v", err)
	}
	refuses(t, repo, ticket, `unknown Milestone "zz"`, "milestone", "set", "mine:init", "zz", "--design", "d2")
	refuses(t, repo, ticket, "does not exist", "milestone", "set", "mine:init", "m2", "--design", "missing")
}

func TestMilestoneSetDesign_AcceptsProjectEpicOfTheInitiativesProject(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, milestoneBody)
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "d2"))

	if _, err := run(t, repo, "milestone", "set", "mine:init", "m2", "--design", "mine:d2"); err != nil {
		t.Fatal(err)
	}
	if got := milestoneBySlug(t, ticket, "m2").Design; got != "d2" {
		t.Errorf("m2 design = %q, want the bare name", got)
	}
	refuses(t, repo, ticket, `in project "other"`, "milestone", "set", "mine:init", "m3", "--design", "other:d2")
}

func TestMilestoneClearDesign_BackToSketchAndDropsTheBrokenMarker(t *testing.T) {
	repo, project := setup(t)
	// d1 is not in the store, so m1 is broken.
	ticket := seedInitiative(t, project, strings.Replace(milestoneBody, "## Gate notes\n\n### m3\noutcome: continue\n", "", 1))

	if _, err := run(t, repo, "milestone", "set", "mine:init", "m1", "--clear-design"); err != nil {
		t.Fatal(err)
	}
	if got := milestoneBySlug(t, ticket, "m1").Design; got != "" {
		t.Errorf("design = %q", got)
	}
	view := initiative.Read(testEpic(readFile(t, ticket)), nil, nil)
	for _, m := range view.Milestones {
		if m.Slug == "m1" && (m.Broken || m.State != initiative.MilestoneSketch) {
			t.Errorf("m1 = %+v, want a sketch", m)
		}
	}
}

func TestMilestoneClearDesign_RefusedWithAGateNote(t *testing.T) {
	repo, project := setup(t)
	body := strings.Replace(milestoneBody, "- m3 | Three | goal: g3", "- m3 | Three | goal: g3 | design: d3", 1)
	ticket := seedInitiative(t, project, body)
	refuses(t, repo, ticket, "gate note", "milestone", "set", "mine:init", "m3", "--clear-design")
}

func TestMilestoneRename_ChangesNameAndGoalAndRefusesASlugChangeOnceNamed(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, milestoneBody)

	if _, err := run(t, repo, "milestone", "set", "mine:init", "m1", "--name", "Uno", "--goal", "new goal"); err != nil {
		t.Fatal(err)
	}
	if m := milestoneBySlug(t, ticket, "m1"); m.Name != "Uno" || m.Goal != "new goal" || m.Design != "d1" {
		t.Errorf("m1 = %+v", m)
	}
	refuses(t, repo, ticket, "cannot change", "milestone", "set", "mine:init", "m1", "--slug", "x1")
	refuses(t, repo, ticket, "gate note", "milestone", "set", "mine:init", "m3", "--slug", "x3")
}

func TestMilestoneSlugChange_RewritesTestedByAndToOnly(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, milestoneBody)

	if _, err := run(t, repo, "milestone", "set", "mine:init", "m2", "--slug", "mid"); err != nil {
		t.Fatal(err)
	}
	b := parsedBody(t, ticket)
	if got := b.Assumptions[1].Value.TestedBy; got != "mid" {
		t.Errorf("A1 tested by = %q", got)
	}
	if got := b.Assumptions[2].Value.TestedBy; got != "m3" {
		t.Errorf("A2 tested by = %q", got)
	}
	if got := b.Deferred[1].Value.To; got != "mid" {
		t.Errorf("one to = %q", got)
	}
	if got := b.Deferred[2].Value.To; got != "m3" {
		t.Errorf("two to = %q", got)
	}
	if b.Assumptions[0].Raw != unparsedAssumption || b.Deferred[0].Raw != unparsedDeferred || b.Milestones[0].Raw != unparsedMilestone {
		t.Error("an unparsed line changed or moved")
	}
	if got := milestoneBySlug(t, ticket, "mid").Name; got != "Two" {
		t.Errorf("name = %q", got)
	}
}

func TestMilestoneSlugChange_RefusesACollision(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, milestoneBody)
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "init-taken-impl"))

	refuses(t, repo, ticket, `Milestone "m3" already exists`, "milestone", "set", "mine:init", "m2", "--slug", "m3")
	refuses(t, repo, ticket, `epic "init-taken-impl" already exists`, "milestone", "set", "mine:init", "m2", "--slug", "taken")
}

func TestMilestoneSet_RefusesNothingToChange(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, milestoneBody)
	refuses(t, repo, ticket, "nothing to change", "milestone", "set", "mine:init", "m2")
}
