package initiatives

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/initiative"
)

const unparsedAssumption = "- hand edited assumption"
const unparsedDeferred = "- hand edited deferred"

const itemsBody = "## Initial prompt\n\nidea\n\n" +
	"## Milestones\n\n- m1 | One | goal: g1 | design: d1\n- m2 | Two | goal: g2\n- m3 | Three | goal: g3\n\n" +
	"## Assumptions\n\n" + unparsedAssumption + "\n- A1 | first | tested by: m1 | state: unknown | evidence: -\n\n" +
	"## Deferred\n\n" + unparsedDeferred + "\n- old item | to: m2 | why: later | from: d1 | state: open\n"

// seedInitiative writes the Initiative "init" with itemsBody and returns its
// ticket.md path.
func seedInitiative(t *testing.T, project, body string) string {
	t.Helper()
	dir := filepath.Join(project, "init")
	testutil.Mkdir(t, dir)
	testutil.WriteFile(t, dir, "ticket.md", "---\nstatus: open\nkind: initiative\n---\n"+body)
	return filepath.Join(dir, "ticket.md")
}

// seedDesign makes Design d1 for Milestone m1 and its -impl epic, merged or not.
func seedDesign(t *testing.T, project string, merged bool) {
	t.Helper()
	d := filepath.Join(project, "d1")
	testutil.Mkdir(t, d)
	testutil.WriteFile(t, d, "ticket.md", "---\nstatus: open\nkind: map\ninitiative: init\nmilestone: m1\n---\n")
	impl := filepath.Join(project, "d1-impl")
	testutil.Mkdir(t, impl)
	fm := "---\nstatus: open\n"
	if merged {
		fm += "merged_at: 2026-01-01T00:00:00Z\n"
	}
	testutil.WriteFile(t, impl, "ticket.md", fm+"---\n")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// refuses runs args, wants an error containing want, and wants the ticket
// file untouched.
func refuses(t *testing.T, repo, ticket, want string, args ...string) {
	t.Helper()
	before := readFile(t, ticket)
	out, err := run(t, repo, args...)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("%v: err = %v, want containing %q", args, err, want)
	}
	if out != "" || readFile(t, ticket) != before {
		t.Errorf("%v: out = %q or the store changed", args, out)
	}
}

func parsedBody(t *testing.T, ticket string) initiative.Body {
	t.Helper()
	return initiative.ParseBody(strings.SplitN(readFile(t, ticket), "---\n", 3)[2])
}

func TestDeferredAdd_WritesAnOpenLineAndPrintsIt(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, itemsBody)

	out, err := run(t, repo, "deferred", "add", "mine:init", "new item", "--to", "m3", "--why", "too big", "--from", "d1")
	want := "- new item | to: m3 | why: too big | from: d1 | state: open"
	if err != nil || out != want+"\n" {
		t.Fatalf("out = %q err = %v", out, err)
	}
	items := parsedBody(t, ticket).Deferred
	if len(items) != 3 || items[2].Value != (initiative.DeferredItem{Item: "new item", To: "m3", Why: "too big", From: "d1", State: "open"}) {
		t.Errorf("deferred = %+v", items)
	}
	if _, err := run(t, repo, "deferred", "add", "mine:init", "floating", "--to", "unplaced", "--why", "w", "--from", "d1"); err != nil {
		t.Errorf("unplaced: %v", err)
	}
}

func TestDeferredAdd_RefusesBadInputAndChangesNothing(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, itemsBody)
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "plain"))
	add := func(item, to, why string) []string {
		return []string{"deferred", "add", "mine:init", item, "--to", to, "--why", why, "--from", "d1"}
	}
	refuses(t, repo, ticket, "unknown Milestone", add("x", "nope", "w")...)
	refuses(t, repo, ticket, "unknown Milestone", add("x", "Bad Slug", "w")...)
	refuses(t, repo, ticket, "must not contain |", add("x", "m1", "a | b")...)
	refuses(t, repo, ticket, "must not contain |", add("x", "m1", "a\nb")...)
	refuses(t, repo, ticket, "already exists", add("old item", "m1", "w")...)
	plain := filepath.Join(project, "plain", "ticket.md")
	refuses(t, repo, plain, "not an initiative", "deferred", "add", "mine:plain", "x", "--to", "unplaced", "--why", "w", "--from", "d1")
}

func TestDeferredSet_ChangesStateAndPlacement(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, itemsBody)
	get := func() initiative.DeferredItem { return parsedBody(t, ticket).Deferred[1].Value }

	if _, err := run(t, repo, "deferred", "set", "mine:init", "old item", "--state", "taken"); err != nil {
		t.Fatal(err)
	}
	if g := get(); g.State != "taken" || g.To != "m2" || g.Why != "later" {
		t.Errorf("taken: %+v", g)
	}
	if _, err := run(t, repo, "deferred", "set", "mine:init", "old item", "--state", "dropped"); err != nil {
		t.Fatal(err)
	}
	if g := get(); g.State != "dropped" {
		t.Errorf("dropped: %+v", g)
	}
	out, err := run(t, repo, "deferred", "set", "mine:init", "old item", "--state", "open", "--to", "m3", "--why", "new reason")
	if err != nil || out != "- old item | to: m3 | why: new reason | from: d1 | state: open\n" {
		t.Fatalf("out = %q err = %v", out, err)
	}
	if _, err := run(t, repo, "deferred", "set", "mine:init", "old item", "--to", "unplaced"); err != nil {
		t.Fatal(err)
	}
	if g := get(); g.To != "unplaced" || g.Why != "new reason" {
		t.Errorf("unplaced: %+v", g)
	}
}

func TestDeferredSet_RefusesBadInputAndChangesNothing(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, itemsBody)
	refuses(t, repo, ticket, "no deferred item", "deferred", "set", "mine:init", "missing", "--state", "taken")
	refuses(t, repo, ticket, "not one of", "deferred", "set", "mine:init", "old item", "--state", "done")
	refuses(t, repo, ticket, "unknown Milestone", "deferred", "set", "mine:init", "old item", "--to", "nope")
	refuses(t, repo, ticket, "nothing to change", "deferred", "set", "mine:init", "old item")
}

func TestAssumptionAdd_NumbersFromTheNextFreeSlot(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, itemsBody)

	out, err := run(t, repo, "assumption", "add", "mine:init", "second", "--tested-by", "m2")
	if err != nil || out != "- A2 | second | tested by: m2 | state: unknown | evidence: -\n" {
		t.Fatalf("out = %q err = %v", out, err)
	}
	if _, err := run(t, repo, "assumption", "add", "mine:init", "third", "--tested-by", "m3"); err != nil {
		t.Fatal(err)
	}
	as := parsedBody(t, ticket).Assumptions
	if len(as) != 4 || as[3].Value != initiative.NewAssumption(3, "third", "m3") {
		t.Errorf("assumptions = %+v", as)
	}
	refuses(t, repo, ticket, "unknown Milestone", "assumption", "add", "mine:init", "x", "--tested-by", "nope")
	refuses(t, repo, ticket, "must not contain |", "assumption", "add", "mine:init", "a | b", "--tested-by", "m1")
}

func TestAssumptionSet_ChangesStateAndEvidence(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, itemsBody)

	out, err := run(t, repo, "assumption", "set", "mine:init", "A1", "--state", "confirmed", "--evidence", "saw it")
	if err != nil || out != "- A1 | first | tested by: m1 | state: confirmed | evidence: saw it\n" {
		t.Fatalf("out = %q err = %v", out, err)
	}
	if a := parsedBody(t, ticket).Assumptions[1].Value; a.State != "confirmed" || a.Evidence != "saw it" {
		t.Errorf("assumption = %+v", a)
	}
	refuses(t, repo, ticket, "no Assumption", "assumption", "set", "mine:init", "A9", "--state", "broken")
	refuses(t, repo, ticket, "not one of", "assumption", "set", "mine:init", "A1", "--state", "maybe")
}

func TestAssumptionSet_TestedByMovesOnlyAtAGate(t *testing.T) {
	move := []string{"assumption", "set", "mine:init", "A1", "--tested-by", "m3"}

	t.Run("no Milestone has a Design", func(t *testing.T) {
		repo, project := setup(t)
		ticket := seedInitiative(t, project, strings.Replace(itemsBody, " | design: d1", "", 1))
		refuses(t, repo, ticket, "no Milestone has a Design", move...)
	})
	t.Run("latest Milestone not merged", func(t *testing.T) {
		repo, project := setup(t)
		ticket := seedInitiative(t, project, itemsBody)
		seedDesign(t, project, false)
		refuses(t, repo, ticket, "not merged", move...)
	})
	t.Run("latest Milestone has a gate note", func(t *testing.T) {
		repo, project := setup(t)
		ticket := seedInitiative(t, project, itemsBody+"\n## Gate notes\n\n### m1\noutcome: continue\n")
		seedDesign(t, project, true)
		refuses(t, repo, ticket, "gate note", move...)
	})
	t.Run("at a Gate", func(t *testing.T) {
		repo, project := setup(t)
		ticket := seedInitiative(t, project, itemsBody)
		seedDesign(t, project, true)
		if _, err := run(t, repo, move...); err != nil {
			t.Fatal(err)
		}
		if a := parsedBody(t, ticket).Assumptions[1].Value; a.TestedBy != "m3" {
			t.Errorf("assumption = %+v", a)
		}
		refuses(t, repo, ticket, "unknown Milestone", "assumption", "set", "mine:init", "A1", "--tested-by", "nope")
	})
}

func TestItems_KeepUnparsedLinesAndOtherSectionsVerbatim(t *testing.T) {
	for _, args := range [][]string{
		{"deferred", "add", "mine:init", "n", "--to", "m1", "--why", "w", "--from", "d1"},
		{"deferred", "set", "mine:init", "old item", "--state", "taken"},
		{"assumption", "add", "mine:init", "n", "--tested-by", "m1"},
		{"assumption", "set", "mine:init", "A1", "--state", "broken"},
	} {
		repo, project := setup(t)
		ticket := seedInitiative(t, project, itemsBody)
		if _, err := run(t, repo, args...); err != nil {
			t.Fatal(err)
		}
		got := readFile(t, ticket)
		for _, section := range []string{initiative.InitialPrompt, initiative.Milestones} {
			if schemaSection(got, section) != schemaSection(itemsBody, section) {
				t.Errorf("%v: section %s changed", args, section)
			}
		}
		for _, line := range []string{unparsedAssumption, unparsedDeferred} {
			if !strings.Contains(got, "\n"+line+"\n") {
				t.Errorf("%v: lost %q", args, line)
			}
		}
		b := parsedBody(t, ticket)
		if b.Assumptions[0].Raw != unparsedAssumption || b.Deferred[0].Raw != unparsedDeferred {
			t.Errorf("%v: unparsed lines moved", args)
		}
	}
}

func schemaSection(body, heading string) string {
	_, rest, _ := strings.Cut(body, "## "+heading+"\n")
	section, _, _ := strings.Cut(rest, "\n## ")
	return section
}

func TestItems_ConcurrentVerbsBothApply(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, itemsBody)
	var wg sync.WaitGroup
	for _, item := range []string{"alpha", "beta"} {
		wg.Go(func() {
			if _, err := run(t, repo, "deferred", "add", "mine:init", item, "--to", "m1", "--why", "w", "--from", "d1"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n := len(parsedBody(t, ticket).Deferred); n != 4 {
		t.Errorf("deferred lines = %d, want 4", n)
	}
}

func TestItems_TicketsSectionStillReadsAndWritesDeferred(t *testing.T) {
	repo, project := setup(t)
	ticket := seedInitiative(t, project, itemsBody)
	if _, err := run(t, repo, "deferred", "set", "mine:init", "old item", "--state", "taken"); err != nil {
		t.Fatal(err)
	}
	if err := tickets.SetEpicSection(ticket, initiative.Deferred, "- replaced"); err != nil {
		t.Fatal(err)
	}
	if got := tickets.LoadEpicTicketMD(filepath.Dir(ticket)).Body; !strings.Contains(got, "## Deferred\n\n- replaced") {
		t.Errorf("body = %q", got)
	}
}
