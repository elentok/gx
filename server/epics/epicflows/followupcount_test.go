package epicflows_test

import (
	"net/http"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/triage"
)

func TestEpicList_FollowUpCountCarriesTheSplitTheTriageStateAndWhatIsLeft(t *testing.T) {
	t.Parallel()
	var rs *triage.Store
	var store string
	loop := "---\nstatus: open\n---\n"
	h, base := rigs.StartEpicDetail(t, func(s string) {
		store = s
		rigs.WriteEpicFiles(t, s, "follow-ups", loop, map[string]string{
			"01-a.md": rigs.IssueMD("01", "open", "category: bug\n", "A"),
			"02-b.md": rigs.IssueMD("02", "open", "category: idea\n", "B"),
			"03-c.md": rigs.IssueMD("03", "draft", "", "C"),
		})
		rs = triage.NewStore(filepath.Join(s, "proj"))
	})
	count := func() (wire.FollowUpCount, bool) {
		t.Helper()
		code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
		for _, f := range rigs.GetEpicList(t, code, body).FollowUps {
			if f.Project == "proj" {
				return f, true
			}
		}
		return wire.FollowUpCount{}, false
	}

	c, ok := count()
	if !ok || c.Bugs != 1 || c.Ideas != 1 || c.Other != 1 || c.Open != 3 || c.Triage != wire.FollowUpTriageNone || c.Left != 0 {
		t.Fatalf("before any Triage: %+v, %v", c, ok)
	}

	rec, err := rs.Create()
	if err != nil {
		t.Fatal(err)
	}
	rec.Step = tickets.StepPlan
	// The snapshot holds 01, 02 and 04 (moved away since); 03 was filed after it.
	rec.Rows = []triage.Row{
		{Address: "proj:follow-ups/01"}, {Address: "proj:follow-ups/02"}, {Address: "proj:follow-ups/04"},
	}
	if err := rs.Save(rec); err != nil {
		t.Fatal(err)
	}
	if c, _ = count(); c.Triage != wire.FollowUpTriageStopped || c.Left != 2 {
		t.Errorf("live, no session: %+v, want stopped with 2 left", c)
	}

	rec, _ = rs.LoadLive()
	rec.Step = tickets.StepApprove
	if err := rs.Save(rec); err != nil {
		t.Fatal(err)
	}
	if c, _ = count(); c.Triage != wire.FollowUpTriageWaiting {
		t.Errorf("at Approve: %+v, want waiting", c)
	}

	// Every follow-up written: a live Triage at zero stays listed.
	rigs.WriteEpicFiles(t, store, "follow-ups", loop, map[string]string{
		"01-a.md": rigs.IssueMD("01", "done", "", "A"),
		"02-b.md": rigs.IssueMD("02", "done", "", "B"),
		"03-c.md": rigs.IssueMD("03", "done", "", "C"),
	})
	if !servertest.Poll(5e9, func() bool { c, ok = count(); return ok && c.Open == 0 }) {
		t.Fatalf("live Triage at zero open is not listed: %+v, %v", c, ok)
	}
	if c.Triage != wire.FollowUpTriageWaiting || c.Left != 0 {
		t.Errorf("at zero: %+v", c)
	}
}

func TestWorkspaces_FollowUpsCarryTheirProjectsWorkspaces(t *testing.T) {
	t.Parallel()
	r := startWorkspaceRig(t)
	ctx := t.Context()
	r.mustWrite(r.h.Client.Admin.CreateWorkspace(ctx, "Work"))
	r.mustMove(ctx, "proj:design", "Work")

	hs, err := r.h.Client.Handshake(ctx)
	if err != nil {
		t.Fatal(err)
	}
	code, body := servertest.WebDo(t, r.h, http.MethodGet, hs.WebURL+"/v1/epics")
	list := rigs.GetEpicList(t, code, body)
	// proj has a Default-workspace epic and a design in Work.
	want := []string{"Default workspace", "Work"}
	sorted := func(ws []string) []string {
		ws = slices.Clone(ws)
		slices.Sort(ws)
		return ws
	}
	if len(list.FollowUps) != 1 {
		t.Fatalf("follow_ups = %+v", list.FollowUps)
	}
	if got := sorted(list.FollowUps[0].Workspaces); !slices.Equal(got, want) {
		t.Errorf("count workspaces = %v, want %v", got, want)
	}
	for _, row := range list.Epics {
		if row.CardKind == wire.CardKindFollowUps {
			if got := sorted(row.Workspaces); !slices.Equal(got, want) {
				t.Errorf("card workspaces = %v, want %v", got, want)
			}
		}
	}
}

func TestWorkspaces_AProjectWithNoCardsIsInTheDefaultWorkspace(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	rigs.WriteEpicFiles(t, store, "follow-ups", "---\nstatus: open\n---\n", map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "F")})
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	code, body := servertest.WebDo(t, h, http.MethodGet, hs.WebURL+"/v1/epics")
	list := rigs.GetEpicList(t, code, body)
	if len(list.FollowUps) != 1 || !slices.Equal(list.FollowUps[0].Workspaces, []string{"Default workspace"}) {
		t.Errorf("follow_ups = %+v, want the Default workspace", list.FollowUps)
	}
}
