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
	if p.Slug != "alpha" || p.State != "active" || !strings.Contains(p.Destination, "Go somewhere.") {
		t.Errorf("header = %+v", p)
	}
	want := []wire.InitiativeMilestone{
		{Number: 1, Slug: "a", Name: "a name", Goal: "goal of a", State: "sketch"},
		{Number: 2, Slug: "b", Name: "b name", Goal: "goal of b", State: "designing", Design: "db", DesignAddress: "proj:db", ImplAddress: "proj:db-impl"},
		{Number: 3, Slug: "c", Name: "c name", Goal: "goal of c", State: "broken", Broken: true, Design: "gone"},
		{Number: 4, Slug: "d", Name: "d name", Goal: "goal of d", State: "designing", Design: "arch", DesignAddress: "proj:arch", ImplAddress: "proj:arch-impl"},
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

func TestInitiatives_PageUnknownOrNotInitiativeIs404(t *testing.T) {
	t.Parallel()
	h, base := startInitiatives(t)
	for _, address := range []string{"proj:nope", "proj:plain", "nope:alpha", "bad"} {
		if code, _ := getInitiative(t, h, base, address); code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", address, code)
		}
	}
}
