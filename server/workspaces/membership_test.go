package workspaces_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/workspaces"
	"github.com/elentok/gx/tickets"
)

// startMembership seeds a Design (map + map-impl), a plain epic, a one-offs
// epic and a follow-ups epic in project proj.
func startMembership(t *testing.T) (servertest.DesignsFixture, context.Context) {
	t.Helper()
	store := t.TempDir()
	for _, epic := range []string{"map", "map-impl", "plain", "one-offs", "follow-ups"} {
		servertest.WriteTicket(t, store, "proj", epic, "01", "first", "")
	}
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := servertest.StartDesignsIn(t, store)
	if _, err := f.H.Client.Admin.CreateWorkspace(f.Ctx, "Bugs"); err != nil {
		t.Fatal(err)
	}
	return f, f.Ctx
}

func TestWorkspacesRoutes_MoveTakesManyTargetsAndRefusesOneAlone(t *testing.T) {
	f, ctx := startMembership(t)
	res, err := f.H.Client.Admin.MoveToWorkspace(ctx, "Bugs", "proj:plain", "proj:nope", "proj:map")
	if err != nil || len(res.Results) != 3 {
		t.Fatalf("move = %+v, %v, want three results", res, err)
	}
	want := []wire.TargetResult{
		{Target: "proj:plain", Status: wire.TargetDone},
		{Target: "proj:nope", Status: wire.TargetRefused, Reason: wire.ReasonWorkspaceUnknownKey},
		{Target: "proj:map", Status: wire.TargetDone},
	}
	for i, r := range res.Results {
		if r.Target != want[i].Target || r.Status != want[i].Status || r.Reason != want[i].Reason {
			t.Errorf("result %d = %+v, want %+v", i, r, want[i])
		}
	}
}

func TestWorkspacesRoutes_ReorderChangesOrderAndRejectsPartialOrUnknown(t *testing.T) {
	h := servertest.Start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, n := range []string{"A", "B"} {
		if _, err := h.Client.Admin.CreateWorkspace(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	res, err := h.Client.Admin.ReorderWorkspaces(ctx, []string{"B", workspaces.DefaultName, "A"})
	if err != nil || res.Refused {
		t.Fatalf("reorder = %+v, %v", res, err)
	}
	if want := []string{"B", workspaces.DefaultName, "A"}; !slices.Equal(names(res.Workspaces), want) {
		t.Errorf("order = %v, want %v", names(res.Workspaces), want)
	}
	for name, order := range map[string][]string{
		"partial":   {"A", "B"},
		"duplicate": {"A", "A", "B"},
		"unknown":   {"A", "B", "Nope"},
	} {
		res, err := h.Client.Admin.ReorderWorkspaces(ctx, order)
		if err != nil || !res.Refused {
			t.Errorf("%s: result = %+v, %v, want refused", name, res, err)
		}
	}
	l, _ := h.Client.Reads.Workspaces(ctx)
	if want := []string{"B", workspaces.DefaultName, "A"}; !slices.Equal(names(l.Workspaces), want) {
		t.Errorf("order after refusals = %v, want %v", names(l.Workspaces), want)
	}
}

func TestWorkspacesRoutes_MoveAndRemoveMembers(t *testing.T) {
	f, ctx := startMembership(t)
	chat, err := f.H.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir()})
	if err != nil || chat.Session == nil {
		t.Fatalf("create chat = %+v, %v", chat, err)
	}
	counts := func() int {
		l, _ := f.H.Client.Reads.Workspaces(ctx)
		for _, w := range l.Workspaces {
			if w.Name == "Bugs" {
				return w.Members
			}
		}
		return -1
	}
	for _, key := range []string{"proj:map", "proj:plain", chat.Session.ID, "proj:map-impl"} {
		res, err := f.H.Client.Admin.MoveToWorkspace(ctx, "Bugs", key)
		if err != nil || res.Results[0].Refused() {
			t.Fatalf("move %s = %+v, %v", key, res, err)
		}
	}
	// The `-impl` address moved the Design, so it is one member with `proj:map`.
	if got := counts(); got != 3 {
		t.Errorf("Bugs members = %d, want 3 (Design, epic, chat)", got)
	}
	if res, err := f.H.Client.Admin.RemoveFromWorkspace(ctx, "proj:map-impl"); err != nil || res.Refused {
		t.Fatalf("remove via -impl = %+v, %v", res, err)
	}
	if got := counts(); got != 2 {
		t.Errorf("Bugs members after remove = %d, want 2", got)
	}
	// A Default member is a no-op, not a refusal.
	if res, err := f.H.Client.Admin.RemoveFromWorkspace(ctx, "proj:map"); err != nil || res.Refused {
		t.Errorf("remove a Default member = %+v, %v, want a no-op success", res, err)
	}
}

func TestWorkspacesRoutes_MoveRefusals(t *testing.T) {
	f, ctx := startMembership(t)
	fix, err := f.H.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Ticket: "proj:plain/01", FixOf: "proj:plain/01"})
	if err != nil || fix.Session == nil {
		t.Fatalf("create fix = %+v, %v", fix, err)
	}
	projectFix, err := f.H.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), FixOf: "proj"})
	if err != nil || projectFix.Session == nil {
		t.Fatalf("create project fix = %+v, %v", projectFix, err)
	}
	design := f.Open(t, "proj:map", false)
	if design.Session == nil {
		t.Fatalf("open design = %+v", design)
	}
	tests := []struct {
		name, key, reason string
	}{
		{"Design agent", design.Session.ID, wire.ReasonWorkspaceFollowsEpic},
		{"Fix agent", fix.Session.ID, wire.ReasonWorkspaceNotJoinable},
		{"project-level Fix agent", projectFix.Session.ID, wire.ReasonWorkspaceNotJoinable},
		{"follow-ups epic", "proj:follow-ups", wire.ReasonWorkspaceNotJoinable},
		{"one-offs epic", "proj:one-offs", wire.ReasonWorkspaceNotJoinable},
		{"unknown epic", "proj:nope", wire.ReasonWorkspaceUnknownKey},
		{"unknown chat", "nope", wire.ReasonWorkspaceUnknownKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := f.H.Client.Admin.MoveToWorkspace(ctx, "Bugs", tt.key)
			if err != nil || len(out.Results) != 1 {
				t.Fatalf("move = %+v, %v", out, err)
			}
			res := out.Results[0]
			if !res.Refused() || res.Reason != tt.reason || res.Message == "" || res.Target != tt.key {
				t.Errorf("result = %+v, want refused with %q and a message", res, tt.reason)
			}
		})
	}
}

func TestWorkspacesRoutes_SnapshotPlacesChatsDesignsAndFixAgents(t *testing.T) {
	f, ctx := startMembership(t)
	create := func(req wire.CreateSessionRequest) string {
		req.Project, req.Dir = "proj", t.TempDir()
		res, err := f.H.Client.Sessions.Create(ctx, req)
		if err != nil || res.Session == nil {
			t.Fatalf("create %+v = %+v, %v", req, res, err)
		}
		return res.Session.ID
	}
	chat := create(wire.CreateSessionRequest{})
	fix := create(wire.CreateSessionRequest{Ticket: "proj:map-impl/01", FixOf: "proj:map-impl/01"})
	projectFix := create(wire.CreateSessionRequest{FixOf: "proj"})
	oneOffFix := create(wire.CreateSessionRequest{Ticket: "proj:one-offs/01", FixOf: "proj:one-offs/01"})
	designRes := f.Open(t, "proj:map", false)
	if designRes.Session == nil {
		t.Fatalf("open design = %+v", designRes)
	}
	design := designRes.Session.ID
	if res, err := f.H.Client.Admin.MoveToWorkspace(ctx, "Bugs", chat); err != nil || res.Results[0].Refused() {
		t.Fatalf("move chat = %+v, %v", res, err)
	}
	snap, err := f.H.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]wire.SessionPlacement{}
	for _, p := range snap.Placements {
		got[p.Session] = p
	}
	want := map[string]wire.SessionPlacement{
		chat:       {Session: chat, Kind: wire.PlacementChat, Workspace: "Bugs"},
		fix:        {Session: fix, Kind: wire.PlacementEpic, Epic: "proj:map"},
		projectFix: {Session: projectFix, Kind: wire.PlacementOther},
		oneOffFix:  {Session: oneOffFix, Kind: wire.PlacementOther},
		design:     {Session: design, Kind: wire.PlacementEpic, Epic: "proj:map"},
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("placement of %s = %+v, want %+v", id, got[id], w)
		}
	}
}

type fakeDir struct {
	designs  map[string]bool
	oneOffs  map[string]bool
	sessions []wire.SessionInfo
}

func (d fakeDir) DesignExists(key string) bool { return d.designs[key] }
func (d fakeDir) Sessions() []wire.SessionInfo { return d.sessions }
func (d fakeDir) RootQueued(string) bool       { return false }
func (d fakeDir) StopSession(string) error     { return nil }
func (d fakeDir) Epic(key string) (tickets.Epic, bool) {
	_, name, _ := tickets.ParseEpicAddress(key)
	return tickets.Epic{Name: name, Path: filepath.Join("/store", "proj", name)}, true
}

func TestStorePlace_QueueAndOneOffAgents(t *testing.T) {
	s, err := workspaces.Open(t.TempDir(), broker.New(0), func() workspaces.Directory {
		return fakeDir{designs: map[string]bool{"proj:map": true}}
	})
	if err != nil {
		t.Fatal(err)
	}
	got := s.Place([]wire.SessionInfo{
		{ID: "agent-a", Queue: true, Ticket: "proj:map-impl/02"},
		{ID: "agent-b", Queue: true, Ticket: "proj:plain/01"},
		{ID: "agent-c", Queue: true, Ticket: "proj:one-offs/03"},
	})
	want := []wire.SessionPlacement{
		{Session: "agent-a", Kind: wire.PlacementEpic, Epic: "proj:map"},
		{Session: "agent-b", Kind: wire.PlacementEpic, Epic: "proj:plain"},
		{Session: "agent-c", Kind: wire.PlacementOther},
	}
	if !slices.Equal(got, want) {
		t.Errorf("placements = %+v, want %+v", got, want)
	}
}

func TestStorePlace_TriageSessionSitsUnderOther(t *testing.T) {
	s, err := workspaces.Open(t.TempDir(), broker.New(0), func() workspaces.Directory { return fakeDir{} })
	if err != nil {
		t.Fatal(err)
	}
	got := s.Place([]wire.SessionInfo{{ID: "t1", Flow: "triage", FlowKey: "proj"}})
	want := []wire.SessionPlacement{{Session: "t1", Kind: wire.PlacementOther}}
	if !slices.Equal(got, want) {
		t.Errorf("placements = %+v, want %+v", got, want)
	}
}

func TestStorePlace_ArchiveKeysPlaceAsEpicAndOthersAreUnchanged(t *testing.T) {
	s, err := workspaces.Open(t.TempDir(), broker.New(0), func() workspaces.Directory {
		return fakeDir{designs: map[string]bool{"proj:map": true}}
	})
	if err != nil {
		t.Fatal(err)
	}
	got := s.Place([]wire.SessionInfo{
		{ID: "design", Design: true, Ticket: "proj:.archive/foo-2"},
		{ID: "fix", FixOf: "proj:.archive/foo-2/01"},
		{ID: "queue", Queue: true, Ticket: "proj:.archive/foo-2/01"},
		{ID: "project-fix", FixOf: "proj"},
		{ID: "one-off", Queue: true, Ticket: "proj:one-offs/03"},
		{ID: "chat"},
		{ID: "impl", Queue: true, Ticket: "proj:map-impl/02"},
	})
	want := []wire.SessionPlacement{
		{Session: "design", Kind: wire.PlacementEpic, Epic: "proj:.archive/foo-2"},
		{Session: "fix", Kind: wire.PlacementEpic, Epic: "proj:.archive/foo-2"},
		{Session: "queue", Kind: wire.PlacementEpic, Epic: "proj:.archive/foo-2"},
		{Session: "project-fix", Kind: wire.PlacementOther},
		{Session: "one-off", Kind: wire.PlacementOther},
		{Session: "chat", Kind: wire.PlacementChat, Workspace: s.Workspace("chat")},
		{Session: "impl", Kind: wire.PlacementEpic, Epic: "proj:map"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("placements = %+v, want %+v", got, want)
	}
}

func TestWorkspacesRoutes_ArchiveKeySessionPlacesAsEpicWithNoRow(t *testing.T) {
	f, ctx := startMembership(t)
	res, err := f.H.Client.Sessions.Create(ctx, wire.CreateSessionRequest{
		Project: "proj", Dir: t.TempDir(), Ticket: "proj:.archive/foo-2/01", FixOf: "proj:.archive/foo-2/01",
	})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	snap, err := f.H.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := wire.SessionPlacement{Session: res.Session.ID, Kind: wire.PlacementEpic, Epic: "proj:.archive/foo-2"}
	found := false
	for _, p := range snap.Placements {
		if p.Session == want.Session {
			found = true
			if p != want {
				t.Errorf("placement = %+v, want %+v", p, want)
			}
		}
		if p.Kind == wire.PlacementOther {
			t.Errorf("Other holds %+v, want empty", p)
		}
	}
	if !found {
		t.Errorf("no placement for %s in %+v", want.Session, snap.Placements)
	}
}

// lockProbeDir tries a store write from inside Epic; a write that completes
// there means Join checked the epic without holding the store lock.
type lockProbeDir struct {
	fakeDir
	probe func() bool
}

func (d lockProbeDir) Epic(key string) (tickets.Epic, bool) {
	d.probe()
	return d.fakeDir.Epic(key)
}

func TestStoreJoin_ChecksEpicUnderTheStoreLock(t *testing.T) {
	var s *workspaces.Store
	wroteDuringCheck := false
	probe := func() bool {
		done := make(chan struct{})
		go func() { _ = s.Remove("x"); close(done) }()
		select {
		case <-done:
			wroteDuringCheck = true
		case <-time.After(50 * time.Millisecond):
		}
		return wroteDuringCheck
	}
	var err error
	s, err = workspaces.Open(t.TempDir(), broker.New(0), func() workspaces.Directory {
		return lockProbeDir{fakeDir: fakeDir{}, probe: probe}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Create("Bugs"); err != nil {
		t.Fatal(err)
	}
	if err := s.Join("proj:plain", "Bugs"); err != nil {
		t.Fatal(err)
	}
	if wroteDuringCheck {
		t.Error("a store write completed while Join was checking the epic: check and write do not share one lock hold")
	}
}

func TestStoreJoin_QueueAndOneOffAgentsRefuse(t *testing.T) {
	s, err := workspaces.Open(t.TempDir(), broker.New(0), func() workspaces.Directory {
		return fakeDir{sessions: []wire.SessionInfo{
			{ID: "agent-a", Queue: true, Ticket: "proj:plain/01"},
			{ID: "agent-c", Queue: true, Ticket: "proj:one-offs/03"},
		}}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Create("Bugs"); err != nil {
		t.Fatal(err)
	}
	if err := s.Join("agent-a", "Bugs"); err != workspaces.ReasonFollowsEpic {
		t.Errorf("Queue agent: err = %v, want ReasonFollowsEpic", err)
	}
	if err := s.Join("agent-c", "Bugs"); err != workspaces.ReasonNotJoinable {
		t.Errorf("one-off agent: err = %v, want ReasonNotJoinable", err)
	}
}
