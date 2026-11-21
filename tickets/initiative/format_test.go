package initiative

import (
	"strings"
	"testing"

	"github.com/elentok/gx/tickets/schema"
)

const (
	milestonesText = "- walking-skeleton | Walking skeleton | goal: Ship the thinnest path | design: skeleton-design\n" +
		"- polish | Polish | goal: Make it nice"
	assumptionsText = "- A1 | Users want it | tested by: walking-skeleton | state: unknown | evidence: -\n" +
		"- A2 | It is fast | tested by: polish | state: confirmed | evidence: bench run 12\n" +
		"- A3 | No lock needed | tested by: polish | state: broken | evidence: raced twice"
	deferredText = "- Dark mode | to: polish | why: out of scope | from: skeleton-design | state: open\n" +
		"- Export | to: unplaced | why: unclear | from: skeleton-design | state: taken\n" +
		"- Themes | to: unplaced | why: too big | from: skeleton-design | state: dropped"
	gateText = "### walking-skeleton\n" +
		"outcome: continue\n" +
		"A1 | confirmed | users asked twice\n" +
		"taught: the path is short\n" +
		"placed: Export goes to polish\n" +
		"\n" +
		"### polish\n" +
		"outcome: done\n" +
		"A2 | broken | too slow\n" +
		"taught: measure first\n" +
		"placed: none"
)

func TestRoundTrip(t *testing.T) {
	t.Run("milestones", func(t *testing.T) {
		es := ParseMilestones(milestonesText)
		if len(es) != 2 || es[0].Unparsed() || es[1].Unparsed() {
			t.Fatalf("entries = %+v", es)
		}
		want := Milestone{Slug: "walking-skeleton", Name: "Walking skeleton", Goal: "Ship the thinnest path", Design: "skeleton-design"}
		if es[0].Value != want || es[1].Value.Design != "" || es[1].Value.Slug != "polish" {
			t.Fatalf("values = %+v", es)
		}
		assertRender(t, milestonesText, mustRender(t, RenderMilestones, es))
	})
	t.Run("assumptions", func(t *testing.T) {
		es := ParseAssumptions(assumptionsText)
		if len(es) != 3 || es[0].Value.State != StateUnknown || es[0].Value.Evidence != "-" ||
			es[1].Value.State != StateConfirmed || es[2].Value.State != StateBroken {
			t.Fatalf("entries = %+v", es)
		}
		assertRender(t, assumptionsText, mustRender(t, RenderAssumptions, es))
	})
	t.Run("deferred", func(t *testing.T) {
		es := ParseDeferred(deferredText)
		if len(es) != 3 || es[0].Value.To != "polish" || es[1].Value.To != Unplaced ||
			es[0].Value.State != DeferredOpen || es[1].Value.State != DeferredTaken || es[2].Value.State != DeferredDropped {
			t.Fatalf("entries = %+v", es)
		}
		assertRender(t, deferredText, mustRender(t, RenderDeferredItems, es))
	})
	t.Run("gate notes", func(t *testing.T) {
		es := ParseGateNotes(gateText)
		if len(es) != 10 {
			t.Fatalf("len = %d: %+v", len(es), es)
		}
		for _, e := range es {
			if e.Unparsed() {
				t.Fatalf("unparsed %q", e.Raw)
			}
		}
		assertRender(t, gateText, mustRender(t, RenderGateNotes, es))
	})
	t.Run("all outcomes parse, unknown outcome is unparsed", func(t *testing.T) {
		for _, o := range []string{"continue", "reshape", "stop", "done"} {
			if es := ParseGateNotes("outcome: " + o); es[0].Unparsed() || es[0].Value.Outcome != o {
				t.Errorf("%s: %+v", o, es)
			}
		}
		if es := ParseGateNotes("outcome: maybe"); !es[0].Unparsed() || es[0].Raw != "outcome: maybe" {
			t.Errorf("maybe: %+v", es)
		}
	})
}

func TestParse_MalformedLinesStayUnparsed(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		parse func(string) (total, unparsed int, raws []string)
		bad   []string
	}{
		{"milestones", milestonesText, countMilestones, []string{
			"- only-slug | Name", "- Bad Slug | Name | goal: g", "- a | b | goal: g | design: ", "a | b | goal: g", "free text",
		}},
		{"assumptions", assumptionsText, countAssumptions, []string{
			"- A1 | text | tested by: x | state: unknown", "- A4 | text | tested by: x | state: maybe | evidence: -",
			"- text | tested by: x | state: unknown | evidence: -", "- A5 ; text ; tested by: x ; state: unknown ; evidence: -", "free text",
		}},
		{"deferred", deferredText, countDeferred, []string{
			"- item | to: polish | why: r | from: d", "- item | to: polish | why: r | from: d | state: lost",
			"- item | to: Bad Slug | why: r | from: d | state: open", "- item ; to: x ; why: r ; from: d ; state: open", "free text",
		}},
		{"gate notes", gateText, countGate, []string{
			"outcome: maybe", "A1 | confirmed", "A1 | fine | evidence", "### Bad Slug", "free text",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			goodTotal, _, _ := c.parse(c.text)
			for _, bad := range c.bad {
				total, unparsed, raws := c.parse(c.text + "\n" + bad)
				if total != goodTotal+1 || unparsed != 1 || raws[0] != bad {
					t.Errorf("%q: total=%d unparsed=%d raws=%q", bad, total, unparsed, raws)
				}
			}
		})
	}
}

func TestRender_UnparsedLineStaysVerbatimInPlace(t *testing.T) {
	handEdited := "- A9 -- hand edited,  odd spacing"
	lines := strings.Split(assumptionsText, "\n")
	text := strings.Join([]string{lines[0], handEdited, lines[1], lines[2]}, "\n")
	es := ParseAssumptions(text)
	if len(es) != 4 || !es[1].Unparsed() {
		t.Fatalf("entries = %+v", es)
	}
	es[2].Value.State = StateBroken
	got := mustRender(t, RenderAssumptions, es)
	want := strings.Replace(text, "state: confirmed", "state: broken", 1)
	assertRender(t, want, got)
}

func TestParseBody_MissingAndExtraSections(t *testing.T) {
	b := ParseBody("## Initial prompt\n\nbuild it\n")
	if len(b.Milestones)+len(b.Assumptions)+len(b.Deferred)+len(b.GateNotes) != 0 {
		t.Fatalf("expected empty: %+v", b)
	}
	got, err := b.Render()
	if err != nil || got != "## Initial prompt\n\nbuild it\n" {
		t.Fatalf("render = %q, %v", got, err)
	}

	body := "## Initial prompt\n\nbuild it\n\n## Notes\n\nkeep me\n\n## Milestones\n\n" + milestonesText + "\n"
	b = ParseBody(body)
	got, err = b.Render()
	if err != nil || got != body {
		t.Fatalf("unchanged render = %q, %v", got, err)
	}
	b.Assumptions = append(b.Assumptions, Parsed(NewAssumption(1, "It works", "polish")))
	got, err = b.Render()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "## Notes\n\nkeep me\n") || !strings.Contains(got, "## Assumptions\n\n- A1 | It works | tested by: polish | state: unknown | evidence: -\n") {
		t.Fatalf("render = %q", got)
	}
}

func TestRender_RefusesBrokenFields(t *testing.T) {
	okA := NewAssumption(1, "text", "slug")
	okD := DeferredItem{Item: "i", To: "slug", Why: "w", From: "f", State: DeferredOpen}
	bad := func(a Assumption, mut func(*Assumption)) Assumption { mut(&a); return a }
	badD := func(mut func(*DeferredItem)) DeferredItem { d := okD; mut(&d); return d }

	cases := []struct {
		name  string
		err   error
		field string
	}{
		{"pipe", errOf(RenderAssumption(bad(okA, func(a *Assumption) { a.Text = "a | b" }))), "text"},
		{"newline", errOf(RenderAssumption(bad(okA, func(a *Assumption) { a.Evidence = "a\nb" }))), "evidence"},
		{"bad state", errOf(RenderAssumption(bad(okA, func(a *Assumption) { a.State = "maybe" }))), "state"},
		{"empty required", errOf(RenderAssumption(bad(okA, func(a *Assumption) { a.Text = "" }))), "text"},
		{"non-slug", errOf(RenderAssumption(bad(okA, func(a *Assumption) { a.TestedBy = "Not A Slug" }))), "tested by"},
		{"bad to", errOf(RenderDeferred(badD(func(d *DeferredItem) { d.To = "to somewhere" }))), "to"},
		{"bad deferred state", errOf(RenderDeferred(badD(func(d *DeferredItem) { d.State = "lost" }))), "state"},
		{"milestone slug", errOf(RenderMilestone(Milestone{Slug: "Bad", Name: "n", Goal: "g"})), "slug"},
		{"milestone goal", errOf(RenderMilestone(Milestone{Slug: "ok", Name: "n"})), "goal"},
		{"gate outcome", errOf(RenderGateLine(GateLine{Kind: GateOutcome, Outcome: "maybe"})), "outcome"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.err == nil || !strings.Contains(c.err.Error(), c.field) {
				t.Fatalf("err = %v, want one naming %q", c.err, c.field)
			}
		})
	}

	if _, err := RenderDeferred(badD(func(d *DeferredItem) { d.To = Unplaced })); err != nil {
		t.Errorf("unplaced refused: %v", err)
	}
	if _, err := RenderAssumptions([]Entry[Assumption]{Parsed(okA), Parsed(okA)}); err == nil {
		t.Error("duplicate A1 accepted")
	}

	line, err := RenderAssumption(okA)
	if err != nil {
		t.Fatal(err)
	}
	if es := ParseAssumptions(line); len(es) != 1 || es[0].Unparsed() || es[0].Value != okA {
		t.Errorf("accepted line did not parse back: %+v", es)
	}
}

func TestNextAssumptionNumber(t *testing.T) {
	if got := NextAssumptionNumber(nil); got != 1 {
		t.Errorf("none = %d", got)
	}
	text := assumptionsText + "\n- A99 | hand edited | tested by: x | state: nope | evidence: -"
	es := ParseAssumptions(text)
	if got := NextAssumptionNumber(es); got != 4 {
		t.Errorf("got %d, want 4 (unparsed A99 must not count)", got)
	}
}

func TestParseBody_AfterSetSectionByHand(t *testing.T) {
	body := "## Initial prompt\n\np\n\n## Destination\n\nd\n\n## Milestones\n\n" + milestonesText +
		"\n\n## Assumptions\n\n" + assumptionsText + "\n\n## Deferred\n\nold\n\n## Gate notes\n\n" + gateText + "\n"
	hand := "- Fast path | to: unplaced | why: r | from: d | state: open\nnot a deferred line"
	body = schema.SetSection(body, Deferred, hand)

	b := ParseBody(body)
	if len(b.Deferred) != 2 || b.Deferred[0].Unparsed() || !b.Deferred[1].Unparsed() || b.Deferred[1].Raw != "not a deferred line" {
		t.Fatalf("deferred = %+v", b.Deferred)
	}
	if len(b.Milestones) != 2 || len(b.Assumptions) != 3 || len(b.GateNotes) != 10 {
		t.Fatalf("other sections changed: %+v", b)
	}
	got, err := b.Render()
	if err != nil || got != body {
		t.Fatalf("render = %q, %v", got, err)
	}
}

func mustRender[T any](t *testing.T, render func([]Entry[T]) (string, error), es []Entry[T]) string {
	t.Helper()
	got, err := render(es)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func assertRender(t *testing.T, want, got string) {
	t.Helper()
	if got != want {
		t.Fatalf("render differs\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func errOf(_ string, err error) error { return err }

func tally[T any](es []Entry[T]) (total, unparsed int, raws []string) {
	for _, e := range es {
		total++
		if e.Unparsed() {
			unparsed++
			raws = append(raws, e.Raw)
		}
	}
	return
}

func countMilestones(s string) (int, int, []string)  { return tally(ParseMilestones(s)) }
func countAssumptions(s string) (int, int, []string) { return tally(ParseAssumptions(s)) }
func countDeferred(s string) (int, int, []string)    { return tally(ParseDeferred(s)) }
func countGate(s string) (int, int, []string)        { return tally(ParseGateNotes(s)) }
