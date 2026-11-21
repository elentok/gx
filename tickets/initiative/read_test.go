package initiative

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
)

func ms(slug, design string) string {
	line := "- " + slug + " | " + slug + " name | goal: goal of " + slug
	if design != "" {
		line += " | design: " + design
	}
	return line
}

func initEpic(milestones []string, gates string) tickets.Epic {
	body := "## Milestones\n\n" + strings.Join(milestones, "\n") + "\n"
	if gates != "" {
		body += "\n## Gate notes\n\n" + gates + "\n"
	}
	return tickets.Epic{Name: "ini", IsInitiative: true, Body: body}
}

func design(name, milestone string) tickets.Epic {
	return tickets.Epic{Name: name, IsMap: true, Initiative: "ini", Milestone: milestone}
}

func gate(slug, outcome string) string { return "### " + slug + "\noutcome: " + outcome }

func states(v View) []string {
	var out []string
	for _, m := range v.Milestones {
		out = append(out, m.State)
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestReadMilestoneStates(t *testing.T) {
	init := initEpic([]string{ms("a", ""), ms("b", "db"), ms("c", "dc"), ms("d", "dd"), ms("e", "de")}, gate("e", "continue"))
	live := []tickets.Epic{
		design("db", "b"), design("dc", "c"), {Name: "dc-impl"},
		design("dd", "d"), {Name: "dd-impl", MergedAt: time.Unix(1, 0)}, design("de", "e"),
	}
	v := Read(init, live, nil)
	eq(t, states(v), []string{MilestoneSketch, MilestoneDesigning, MilestoneImplementing, MilestoneMerged, MilestoneGated})
	if m := v.Milestones[0]; m.Design != "" || m.ImplEpic != "" || m.Number != 1 {
		t.Fatalf("sketch = %+v", m)
	}
	if m := v.Milestones[2]; m.ImplEpic != "dc-impl" || m.Number != 3 || m.Name != "c name" || m.Goal != "goal of c" {
		t.Fatalf("implementing = %+v", m)
	}
	if v.Done != 1 || v.Total != 5 || v.Current != "a" {
		t.Fatalf("progress = %d/%d current %q", v.Done, v.Total, v.Current)
	}
}

func TestReadMergedAtBoundary(t *testing.T) {
	init := initEpic([]string{ms("a", "da")}, "")
	live := []tickets.Epic{design("da", "a"), {Name: "da-impl"}}
	eq(t, states(Read(init, live, nil)), []string{MilestoneImplementing})
	live[1].MergedAt = time.Unix(1, 0)
	eq(t, states(Read(init, live, nil)), []string{MilestoneMerged})
	init = initEpic([]string{ms("a", "da")}, gate("a", "continue"))
	eq(t, states(Read(init, live, nil)), []string{MilestoneGated})
}

func TestReadBrokenAndArchived(t *testing.T) {
	init := initEpic([]string{ms("a", "gone")}, "")
	v := Read(init, nil, nil)
	m := v.Milestones[0]
	if !m.Broken || m.Design != "gone" || m.ImplEpic != "" {
		t.Fatalf("broken = %+v", m)
	}
	if len(v.Problems) != 1 || v.Problems[0].Kind != ProblemMissingDesign ||
		v.Problems[0].Milestone != "a" || v.Problems[0].Design != "gone" || !strings.Contains(v.Problems[0].Message, "gone") {
		t.Fatalf("problems = %+v", v.Problems)
	}

	archived := []tickets.Epic{design("gone", "a")}
	v = Read(init, nil, archived)
	if v.Milestones[0].Broken || v.Milestones[0].State != MilestoneDesigning || len(v.Problems) != 0 {
		t.Fatalf("archived design = %+v %+v", v.Milestones[0], v.Problems)
	}
	archived = append(archived, tickets.Epic{Name: "gone-impl", MergedAt: time.Unix(1, 0)})
	eq(t, states(Read(init, nil, archived)), []string{MilestoneMerged})
}

func TestReadInitiativeState(t *testing.T) {
	cases := []struct {
		name  string
		gates string
		want  string
		done  int
		cur   string
	}{
		{"none", "", StateActive, 0, "a"},
		{"continue", gate("a", "continue"), StateActive, 1, "b"},
		{"reshape", gate("a", "reshape"), StateActive, 1, "b"},
		{"stop", gate("a", "continue") + "\n" + gate("b", "stop"), StateStopped, 2, ""},
		{"done", gate("a", "continue") + "\n" + gate("b", "done"), StateFinished, 2, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := Read(initEpic([]string{ms("a", ""), ms("b", "")}, c.gates), nil, nil)
			if v.State != c.want || v.Done != c.done || v.Total != 2 || v.Current != c.cur {
				t.Fatalf("state=%s done=%d/%d current=%q", v.State, v.Done, v.Total, v.Current)
			}
		})
	}
}

func TestBackLink(t *testing.T) {
	init := initEpic([]string{ms("a", "da"), ms("b", "other")}, "")
	good := design("da", "a")
	all := []tickets.Epic{init, good, {Name: "da-impl"}}

	for _, e := range []tickets.Epic{good, all[2]} {
		bl := BackLinkFor(e, all)
		if bl.Milestone != 1 || bl.Initiative != "ini" || bl.Problem != nil {
			t.Fatalf("%s: %+v", e.Name, bl)
		}
	}
	for _, c := range []struct {
		name string
		e    tickets.Epic
		all  []tickets.Epic
		kind string
	}{
		{"initiative missing", good, []tickets.Epic{good}, ProblemUnresolvedInitiative},
		{"milestone unknown", design("dx", "nope"), []tickets.Epic{init}, ProblemUnknownMilestone},
		{"different design", design("dx", "a"), []tickets.Epic{init}, ProblemMismatchedDesign},
	} {
		t.Run(c.name, func(t *testing.T) {
			bl := BackLinkFor(c.e, c.all)
			if bl.Milestone != 0 || bl.Initiative != "" || bl.Problem == nil || bl.Problem.Kind != c.kind {
				t.Fatalf("%+v", bl)
			}
		})
	}
	if bl := BackLinkFor(tickets.Epic{Name: "plain", IsMap: true}, all); bl.Problem != nil || bl.Milestone != 0 {
		t.Fatalf("plain map = %+v", bl)
	}
}

func TestReadLinkProblemsAndPassThrough(t *testing.T) {
	init := initEpic([]string{ms("a", "da"), "garbled milestone line", ms("b", "gone")}, gate("a", "continue")+"\nstray gate text")
	live := []tickets.Epic{design("da", "a"), design("dx", "a"), design("dy", "zzz")}
	v := Read(init, live, nil)

	kinds := map[string]string{}
	for _, p := range v.Problems {
		kinds[p.Kind] = p.Message
	}
	for _, k := range []string{ProblemUnparsedMilestone, ProblemMissingDesign, ProblemMismatchedDesign, ProblemUnknownMilestone} {
		if kinds[k] == "" {
			t.Fatalf("missing problem %s in %+v", k, v.Problems)
		}
	}
	if !strings.Contains(kinds[ProblemUnparsedMilestone], "garbled milestone line") {
		t.Fatalf("message = %q", kinds[ProblemUnparsedMilestone])
	}
	if v.Total != 2 {
		t.Fatalf("other milestones stopped: total %d", v.Total)
	}
	var raws []string
	for _, u := range v.Unparsed {
		raws = append(raws, u.Section+":"+u.Raw)
	}
	eq(t, raws, []string{"Milestones:garbled milestone line", "Gate notes:stray gate text"})

	un := UnresolvedInitiatives([]tickets.Epic{init, {Name: "m", IsMap: true, Initiative: "ghost", Milestone: "a"}, design("da", "a")})
	if len(un) != 1 || un[0].Kind != ProblemUnresolvedInitiative || !strings.Contains(un[0].Message, "ghost") {
		t.Fatalf("unresolved = %+v", un)
	}
}

func TestReadOrphans(t *testing.T) {
	init := initEpic([]string{ms("a", ""), ms("b", "db")}, "")
	live := []tickets.Epic{design("orphan", "a"), design("db", "b")}
	v := Read(init, live, nil)
	if len(v.Orphans) != 1 || v.Orphans[0] != "orphan" {
		t.Fatalf("orphans = %v", v.Orphans)
	}
}
