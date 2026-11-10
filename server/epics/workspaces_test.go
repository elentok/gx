package epics_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

type wsRig struct {
	t *testing.T
	h *servertest.Harness
}

func startWorkspaceRig(t *testing.T) wsRig {
	t.Helper()
	store := t.TempDir()
	mapTickets := map[string]string{"01-a.md": rigs.IssueMD("01", "done", "", "A")}
	implTickets := map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")}
	rigs.WriteEpicFiles(t, store, "plain", "---\nstatus: open\n---\n", map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")})
	rigs.WriteEpicFiles(t, store, "design", joinedMapMD("branch"), mapTickets)
	rigs.WriteEpicFiles(t, store, "design-impl", "---\nstatus: open\n---\n", implTickets)
	rigs.WriteEpicFiles(t, store, "follow-ups", "---\nstatus: open\n---\n", map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "F")})
	servertest.WriteTicketWith(t, store, "proj", "stuck", "01", "broken", servertest.TicketOpts{PauseReason: "iteration-error"})
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	return wsRig{t: t, h: h}
}

func (r wsRig) list() map[string]wire.EpicListRow {
	r.t.Helper()
	hs, err := r.h.Client.Handshake(r.t.Context())
	if err != nil {
		r.t.Fatal(err)
	}
	code, body := servertest.WebDo(r.t, r.h, http.MethodGet, hs.WebURL+"/v1/epics")
	got := map[string]wire.EpicListRow{}
	for _, row := range rigs.GetEpicList(r.t, code, body).Epics {
		got[row.Name] = row
	}
	return got
}

func (r wsRig) mustWrite(res wire.WorkspaceResult, err error) {
	r.t.Helper()
	if err != nil || res.Refused {
		r.t.Fatalf("write: %v %+v", err, res)
	}
}

func (r wsRig) mustMove(ctx context.Context, key, workspace string) {
	r.t.Helper()
	res, err := r.h.Client.Admin.MoveToWorkspace(ctx, workspace, key)
	if err != nil || res.Results[0].Refused() {
		r.t.Fatalf("move: %v %+v", err, res)
	}
}

func TestWorkspaces_CardsNameTheirWorkspace(t *testing.T) {
	t.Parallel()
	r := startWorkspaceRig(t)
	ctx := t.Context()
	r.mustWrite(r.h.Client.Admin.CreateWorkspace(ctx, "Work"))
	got := r.list()
	if got["plain"].Workspace != "Default workspace" || got["design"].Workspace != "Default workspace" {
		t.Errorf("default fallback: plain=%q design=%q", got["plain"].Workspace, got["design"].Workspace)
	}
	if got["follow-ups"].Workspace != "" {
		t.Errorf("follow-ups workspace = %q, want none", got["follow-ups"].Workspace)
	}

	before := got["design"]
	r.mustMove(ctx, "proj:design", "Work")
	after := r.list()["design"]
	if after.Workspace != "Work" {
		t.Errorf("joined card workspace = %q, want Work", after.Workspace)
	}
	before.Workspace = "Work"
	if before.Address != after.Address || before.Section != after.Section || before.StatusWords != after.StatusWords || len(before.Actions) != len(after.Actions) {
		t.Errorf("a move changed another field: before %+v after %+v", before, after)
	}
}

func TestWorkspaces_HeldCardsSitOnlyInOnHoldAndLeaveTheCounts(t *testing.T) {
	t.Parallel()
	r := startWorkspaceRig(t)
	ctx := t.Context()

	if row := r.list()["stuck"]; row.Section != wire.SectionNeedsYou {
		t.Fatalf("stuck section = %q, want needs-you", row.Section)
	}
	snap, err := r.h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.NeedsYouTotal != 1 || needsYou(snap, "Default workspace") != 1 {
		t.Fatalf("before hold: total=%d workspaces=%+v", snap.NeedsYouTotal, snap.Workspaces)
	}

	if res, err := r.h.Client.Admin.HoldEpics(ctx, []string{"proj:stuck"}, true, false); err != nil || res.Results[0].Refused() {
		t.Fatalf("hold: %v %+v", err, res)
	}
	row := r.list()["stuck"]
	if row.Section != wire.SectionOnHold || row.StatePill != "" || row.StatusWords != "Implementing..." {
		t.Errorf("held row = section %q pill %q words %q", row.Section, row.StatePill, row.StatusWords)
	}
	if len(row.Actions) != 1 || row.Actions[0].Kind != wire.CardActionUnhold || row.Actions[0].Label != "Take off hold" || row.Actions[0].Target != "proj:stuck" {
		t.Errorf("held actions = %+v", row.Actions)
	}
	snap, err = r.h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.NeedsYouTotal != 0 || needsYou(snap, "Default workspace") != 0 {
		t.Errorf("after hold: total=%d workspaces=%+v", snap.NeedsYouTotal, snap.Workspaces)
	}
}

func TestWorkspaces_HeldDesignPullsItsImplIntoOnHold(t *testing.T) {
	t.Parallel()
	r := startWorkspaceRig(t)
	if res, err := r.h.Client.Admin.HoldEpics(t.Context(), []string{"proj:design"}, true, false); err != nil || res.Results[0].Refused() {
		t.Fatalf("hold: %v %+v", err, res)
	}
	got := r.list()
	if got["design"].Section != wire.SectionOnHold {
		t.Errorf("design section = %q, want on-hold", got["design"].Section)
	}
	if _, ok := got["design-impl"]; ok {
		t.Error("design-impl has its own row, want it joined")
	}
	if a := got["design"].Actions; len(a) != 1 || a[0].Target != got["design"].Addresses[0] {
		t.Errorf("unhold target = %+v, addresses %v", a, got["design"].Addresses)
	}
}

func TestWorkspaces_OneOffCardCountsInTotalButNoWorkspace(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "one-offs", "01", "stuck one-off", servertest.TicketOpts{PauseReason: "iteration-error"})
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	r := wsRig{t: t, h: h}

	if row := r.list()["stuck one-off"]; row.CardKind != wire.CardKindOneOff || row.Section != wire.SectionNeedsYou {
		t.Fatalf("one-off row = kind %q section %q, want a one-off in needs-you", row.CardKind, row.Section)
	}
	snap, err := h.Client.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if snap.NeedsYouTotal != 1 || needsYou(snap, "Default workspace") != 0 {
		t.Errorf("snapshot: total=%d workspaces=%+v, want total 1 and no workspace count", snap.NeedsYouTotal, snap.Workspaces)
	}
	list, err := h.Client.Reads.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range list.Workspaces {
		if w.NeedsYou != 0 {
			t.Errorf("route workspace %+v counts the one-off card", w)
		}
	}
}

func TestWorkspaces_RestartKeepsTheListAndTheHeldCard(t *testing.T) {
	t.Parallel()
	r := startWorkspaceRig(t)
	ctx := t.Context()
	r.mustWrite(r.h.Client.Admin.CreateWorkspace(ctx, "Work"))
	r.mustMove(ctx, "proj:design", "Work")
	if res, err := r.h.Client.Admin.HoldEpics(ctx, []string{"proj:stuck"}, true, false); err != nil || res.Results[0].Refused() {
		t.Fatalf("hold: %v %+v", err, res)
	}
	before, err := r.h.Client.Reads.Workspaces(ctx)
	if err != nil {
		t.Fatal(err)
	}

	r.h.Restart(t)

	after, err := r.h.Client.Reads.Workspaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before.Workspaces, after.Workspaces) {
		t.Errorf("workspaces after restart = %+v, before %+v", after.Workspaces, before.Workspaces)
	}
	got := r.list()
	if got["stuck"].Section != wire.SectionOnHold {
		t.Errorf("stuck section after restart = %q, want on-hold", got["stuck"].Section)
	}
	if got["design"].Workspace != "Work" {
		t.Errorf("design workspace after restart = %q, want Work", got["design"].Workspace)
	}
}

func needsYou(s wire.Snapshot, workspace string) int {
	for _, w := range s.Workspaces {
		if w.Name == workspace {
			return w.NeedsYou
		}
	}
	return -1
}
