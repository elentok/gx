package epics_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

func startInitiatives(t *testing.T) (*servertest.Harness, string) {
	t.Helper()
	return rigs.StartEpicDetail(t, func(store string) { rigs.SeedInitiatives(t, store) })
}

func TestInitiatives_ListOmitsProjectsWithNone(t *testing.T) {
	t.Parallel()
	h, base := startInitiatives(t)
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/initiatives")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var list wire.InitiativeList
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Projects) != 1 || list.Projects[0].Project != "proj" {
		t.Fatalf("projects = %+v, want only proj", list.Projects)
	}
	got := map[string]wire.InitiativeRow{}
	for _, r := range list.Projects[0].Initiatives {
		got[r.Slug] = r
	}
	want := map[string]wire.InitiativeRow{
		"alpha":    {Address: "proj:alpha", Slug: "alpha", State: "active", Done: 0, Total: 4, Current: "a", CurrentNumber: 1, CurrentState: "sketch"},
		"stopper":  {Address: "proj:stopper", Slug: "stopper", State: "stopped", Done: 2, Total: 2, StoppedAt: 2},
		"finisher": {Address: "proj:finisher", Slug: "finisher", State: "finished", Done: 1, Total: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %+v", got)
	}
	for slug, w := range want {
		if got[slug] != w {
			t.Errorf("%s = %+v, want %+v", slug, got[slug], w)
		}
	}
	if strings.Contains(body, "start") {
		t.Errorf("payload mentions start: %s", body)
	}
}

func getInitiative(t *testing.T, h *servertest.Harness, base, address string) (int, wire.InitiativePage) {
	t.Helper()
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/initiatives/detail?address="+address)
	var p wire.InitiativePage
	if code == http.StatusOK {
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatal(err)
		}
	}
	return code, p
}

func TestInitiatives_PageHasMilestonesAndSections(t *testing.T) {
	t.Parallel()
	h, base := startInitiatives(t)
	code, p := getInitiative(t, h, base, "proj:alpha")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if p.Slug != "alpha" || p.Project != "proj" || p.State != "active" || !strings.Contains(p.Destination, "Go somewhere.") {
		t.Errorf("header = %+v", p)
	}
	want := []wire.InitiativeMilestone{
		{Number: 1, Slug: "a", Name: "a name", Goal: "goal of a", State: "sketch"},
		{Number: 2, Slug: "b", Name: "b name", Goal: "goal of b", State: "designing", Design: "db", DesignAddress: "proj:db", ImplAddress: "proj:db-impl"},
		{Number: 3, Slug: "c", Name: "c name", Goal: "goal of c", State: "broken", Broken: true, Design: "gone"},
		{Number: 4, Slug: "d", Name: "d name", Goal: "goal of d", State: "designing", Design: "arch", DesignAddress: "proj:arch", ImplAddress: "proj:arch-impl", DesignArchived: true},
	}
	if len(p.Milestones) != len(want) {
		t.Fatalf("milestones = %+v", p.Milestones)
	}
	for i, w := range want {
		if p.Milestones[i] != w {
			t.Errorf("milestone %d = %+v, want %+v", i+1, p.Milestones[i], w)
		}
	}
	if len(p.Assumptions) != 1 || p.Assumptions[0].N != 1 || p.Assumptions[0].Text != "Users want it" {
		t.Errorf("assumptions = %+v", p.Assumptions)
	}
	if len(p.Deferred) != 1 || p.Deferred[0].Item != "Export" || p.Deferred[0].State != "open" {
		t.Errorf("deferred = %+v", p.Deferred)
	}
	wantUnparsed := []wire.InitiativeUnparsed{
		{Section: "Milestones", Raw: "not a milestone"},
		{Section: "Assumptions", Raw: "bad assumption"},
		{Section: "Deferred", Raw: "bad deferred"},
		{Section: "Gate notes", Raw: "bad gate line"},
	}
	if len(p.Unparsed) != len(wantUnparsed) {
		t.Fatalf("unparsed = %+v", p.Unparsed)
	}
	for i, w := range wantUnparsed {
		if p.Unparsed[i] != w {
			t.Errorf("unparsed %d = %+v, want %+v", i, p.Unparsed[i], w)
		}
	}
}

func TestInitiatives_PageHasGateNotesAndStoppedMilestone(t *testing.T) {
	t.Parallel()
	h, base := startInitiatives(t)
	code, p := getInitiative(t, h, base, "proj:stopper")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if p.State != "stopped" || p.StoppedAt != 2 || len(p.GateNotes) != 2 ||
		p.GateNotes[0].Slug != "x" || p.GateNotes[0].Outcome != "continue" || p.GateNotes[1].Outcome != "stop" {
		t.Errorf("page = %+v", p)
	}
}

func TestInitiatives_GateLineBeforeTheFirstHeadingIsListedUnparsed(t *testing.T) {
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "stray", "---\nstatus: open\nkind: initiative\n---\n\n## Milestones\n\n- x | x name | goal: gx\n\n## Gate notes\n\ntaught: orphan\n### x\noutcome: continue\n", nil)
	})
	code, p := getInitiative(t, h, base, "proj:stray")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	want := []wire.InitiativeUnparsed{{Section: "Gate notes", Raw: "taught: orphan"}}
	if len(p.Unparsed) != 1 || p.Unparsed[0] != want[0] {
		t.Errorf("unparsed = %+v, want %+v", p.Unparsed, want)
	}
	if len(p.GateNotes) != 1 || p.GateNotes[0].Slug != "x" || len(p.GateNotes[0].Taught) != 0 {
		t.Errorf("gate notes = %+v", p.GateNotes)
	}
}

func TestInitiatives_PageUnknownOrNotInitiativeIs404(t *testing.T) {
	t.Parallel()
	h, base := startInitiatives(t)
	for _, address := range []string{"proj:nope", "proj:plain", "nope:alpha", "bad"} {
		if code, _ := getInitiative(t, h, base, address); code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", address, code)
		}
	}
}

const implLoopMD = "---\nstatus: open\n---\n"

func seedBackLinks(t *testing.T, store string) {
	t.Helper()
	open := map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")}
	initiativeMD := func(extra, milestones string) string {
		return "---\nstatus: open\nkind: initiative\n" + extra + "---\n\n## Destination\n\nGo.\n\n## Milestones\n\n" + milestones
	}
	design := func(name, initiative, milestone string) {
		fm := "---\nstatus: open\nkind: map\n"
		if initiative != "" {
			fm += "initiative: " + initiative + "\n"
		}
		if milestone != "" {
			fm += "milestone: " + milestone + "\n"
		}
		rigs.WriteEpicFiles(t, store, name, fm+"---\n", open)
	}
	rigs.WriteEpicFiles(t, store, "alpha", initiativeMD("", "- a | a name | goal: ga | design: other\n- b | b name | goal: gb | design: ok\n- c | c name | goal: gc | design: arch\n"), nil)
	rigs.WriteEpicFiles(t, store, ".archive/old", initiativeMD("archived_at: 2026-03-06T10:00:00Z\n", "- m | m name | goal: gm | design: ofold\n"), nil)

	design("ok", "alpha", "b")
	rigs.WriteEpicFiles(t, store, "ok-impl", implLoopMD, open)
	design("ofold", "old", "m")
	rigs.WriteEpicFiles(t, store, ".archive/arch", "---\nstatus: open\nkind: map\ninitiative: alpha\nmilestone: c\ncompleted_at: 2026-01-02T10:00:00Z\n---\n", nil)
	rigs.WriteEpicFiles(t, store, "arch-impl", implLoopMD, open)
	design("noinit", "ghost", "b")
	design("unknown", "alpha", "nope")
	design("mismatch", "alpha", "a")
	design("plain", "", "")
	rigs.WriteEpicFiles(t, store, "plain-impl", implLoopMD, open)
}

func listBackLinks(t *testing.T) (wire.EpicList, map[string]wire.EpicListRow) {
	t.Helper()
	h, base := rigs.StartEpicDetail(t, func(store string) { seedBackLinks(t, store) })
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	list := rigs.GetEpicList(t, code, body)
	byName := map[string]wire.EpicListRow{}
	for _, r := range list.Epics {
		byName[r.Name] = r
	}
	return list, byName
}

func TestEpicList_DesignAndImplCardCarryTheBackLink(t *testing.T) {
	t.Parallel()
	_, rows := listBackLinks(t)
	want := wire.BackLink{Milestone: 2, Initiative: "alpha", InitiativeAddress: "proj:alpha"}
	r, ok := rows["ok"]
	if !ok {
		t.Fatalf("no ok row in %+v", rows)
	}
	if r.BackLink != want {
		t.Errorf("ok back-link = %+v, want %+v", r.BackLink, want)
	}
	if len(r.Addresses) != 2 || r.Addresses[1] != "proj:ok-impl" {
		t.Errorf("ok card does not carry its -impl epic: %v", r.Addresses)
	}
	if _, listed := rows["ok-impl"]; listed {
		t.Error("the -impl epic got a row of its own")
	}
}

func TestEpicList_UnresolvedBackLinkIsEmptyWithoutError(t *testing.T) {
	t.Parallel()
	_, rows := listBackLinks(t)
	for _, name := range []string{"noinit", "unknown", "mismatch", "plain"} {
		r, ok := rows[name]
		if !ok {
			t.Errorf("%s is missing from the list", name)
			continue
		}
		if r.BackLink != (wire.BackLink{}) {
			t.Errorf("%s back-link = %+v, want none", name, r.BackLink)
		}
	}
}

func TestEpicList_ArchivedInitiativeAndDesignStillResolveBackLink(t *testing.T) {
	t.Parallel()
	_, rows := listBackLinks(t)
	want := wire.BackLink{Milestone: 1, Initiative: "old", InitiativeAddress: "proj:old"}
	if got := rows["ofold"].BackLink; got != want {
		t.Errorf("design of an archived Initiative back-link = %+v, want %+v", got, want)
	}
}

func TestEpicList_ImplEpicOfAnArchivedDesignCarriesTheBackLink(t *testing.T) {
	t.Parallel()
	_, rows := listBackLinks(t)
	want := wire.BackLink{Milestone: 3, Initiative: "alpha", InitiativeAddress: "proj:alpha"}
	r, ok := rows["arch-impl"]
	if !ok {
		t.Fatalf("no arch-impl row in %+v", rows)
	}
	if r.BackLink != want {
		t.Errorf("arch-impl back-link = %+v, want %+v", r.BackLink, want)
	}
}

func TestEpicList_NeverListsAnInitiativeBackLink(t *testing.T) {
	t.Parallel()
	list, rows := listBackLinks(t)
	for _, name := range []string{"alpha", "old"} {
		if _, ok := rows[name]; ok {
			t.Errorf("Initiative %s has a row", name)
		}
	}
	for _, r := range list.Epics {
		if r.CardKind != wire.CardKindDesign && r.CardKind != wire.CardKindImplementation && r.CardKind != wire.CardKindFollowUps {
			t.Errorf("%s has unexpected card kind %q", r.Name, r.CardKind)
		}
	}
}

func TestEpicDetail_DesignAndImplCarryTheBackLink(t *testing.T) {
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) { seedBackLinks(t, store) })
	want := wire.BackLink{Milestone: 2, Initiative: "alpha", InitiativeAddress: "proj:alpha"}
	for addr, w := range map[string]wire.BackLink{
		"proj:ok": want, "proj:ok-impl": want,
		"proj:noinit": {}, "proj:unknown": {}, "proj:mismatch": {}, "proj:plain": {}, "proj:plain-impl": {},
	} {
		code, d, body := rigs.GetEpicDetail(t, h, base, addr)
		if code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", addr, code, body)
		}
		if d.BackLink != w {
			t.Errorf("%s back-link = %+v, want %+v", addr, d.BackLink, w)
		}
	}
}

func TestEpicDetail_ArchivedDesignStillResolvesBackLink(t *testing.T) {
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) { seedBackLinks(t, store) })
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics/archived/detail?address=proj:arch")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if !strings.Contains(body, `"initiative":"alpha"`) || !strings.Contains(body, `"milestone":3`) {
		t.Errorf("archived detail lacks the back-link: %s", body)
	}
}
