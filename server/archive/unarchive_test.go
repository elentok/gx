package archive_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func (r rig) unarchive(t *testing.T, targets ...string) []wire.TargetResult {
	t.Helper()
	res, err := r.H.Client.Admin.UnarchiveEpics(r.Ctx, targets)
	if err != nil {
		t.Fatal(err)
	}
	return res.Results
}

func (r rig) active(t *testing.T, name string) tickets.Epic {
	t.Helper()
	e := tickets.LoadEpicTicketMD(filepath.Join(r.store, "proj", name))
	if !e.HasTicketMD {
		t.Fatalf("no active epic %s", name)
	}
	return e
}

func (r rig) members(t *testing.T, workspace string) int {
	t.Helper()
	list, err := r.H.Client.Reads.Workspaces(r.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range list.Workspaces {
		if w.Name == workspace {
			return w.Members
		}
	}
	t.Fatalf("no workspace %s", workspace)
	return 0
}

func (r rig) isActive(name string) bool {
	_, err := os.Stat(filepath.Join(r.store, "proj", name))
	return err == nil
}

func (r rig) heldKeys(t *testing.T) []string {
	t.Helper()
	snap, err := r.H.Client.Snapshot(r.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	return snap.Held
}

func TestUnarchive_RoundTripReturnsToItsWorkspaceWithoutHold(t *testing.T) {
	r := start(t, "done")
	if _, err := r.H.Client.Admin.CreateWorkspace(r.Ctx, "Bugs"); err != nil {
		t.Fatal(err)
	}
	if res, err := r.H.Client.Admin.MoveToWorkspace(r.Ctx, "Bugs", "proj:done"); err != nil || res.Results[0].Refused() {
		t.Fatalf("move = %+v, %v", res, err)
	}
	if res, err := r.H.Client.Admin.HoldEpics(r.Ctx, []string{"proj:done"}, true, false); err != nil || res.Results[0].Refused() {
		t.Fatalf("hold = %+v, %v", res, err)
	}
	r.archive(t, false, false, "proj:done")

	got := r.unarchive(t, "proj:done")[0]
	if got.Status != wire.TargetDone || got.NewAddress != "" {
		t.Fatalf("unarchive = %+v", got)
	}
	if len(r.archived(t)) != 0 || !r.isActive("done") {
		t.Fatalf("epic not moved back: archived = %v", r.archived(t))
	}
	if e := r.active(t, "done"); !e.ArchivedAt.IsZero() || e.ArchivedWorkspace != "" || e.BranchName != "" {
		t.Errorf("stamps kept: %v %q %q", e.ArchivedAt, e.ArchivedWorkspace, e.BranchName)
	}
	if n := r.members(t, "Bugs"); n != 1 {
		t.Errorf("Bugs members = %d, want 1", n)
	}
	if held := r.heldKeys(t); slices.Contains(held, "proj:done") {
		t.Errorf("hold mark came back: %v", held)
	}
}

func TestUnarchive_FailedSessionRekeyIsAFailedTarget(t *testing.T) {
	r := start(t, "done")
	id := r.stoppedFixSession(t, "proj:done/01")
	// Archived while writable, so only Unarchive meets the lock.
	r.archive(t, true, false, "proj:done")
	r.lock(t, id)

	got := r.unarchive(t, "proj:done")[0]
	if got.Status != wire.TargetFailed || !strings.Contains(got.Message, "re-key session") {
		t.Fatalf("unarchive = %+v, want a failed target naming the re-key", got)
	}
}

func TestUnarchive_DeletedWorkspaceLandsInDefault(t *testing.T) {
	r := start(t, "done")
	if _, err := r.H.Client.Admin.CreateWorkspace(r.Ctx, "Bugs"); err != nil {
		t.Fatal(err)
	}
	if res, err := r.H.Client.Admin.MoveToWorkspace(r.Ctx, "Bugs", "proj:done"); err != nil || res.Results[0].Refused() {
		t.Fatalf("move = %+v, %v", res, err)
	}
	r.archive(t, false, false, "proj:done")
	if res, err := r.H.Client.Admin.DeleteWorkspace(r.Ctx, "Bugs"); err != nil || res.Refused {
		t.Fatalf("delete = %+v, %v", res, err)
	}

	if got := r.unarchive(t, "proj:done")[0]; got.Status != wire.TargetDone {
		t.Fatalf("unarchive = %+v", got)
	}
	if !r.isActive("done") {
		t.Fatal("epic not moved back")
	}
}

func TestUnarchive_ClashTakesTheNextSuffixAndKeepsTheBranchName(t *testing.T) {
	r := start(t, "foo")
	r.archive(t, false, false, "proj:foo")
	servertest.WriteCompleteEpic(t, r.store, "foo")

	got := r.unarchive(t, "proj:foo")[0]
	if got.Status != wire.TargetDone || got.NewAddress != "proj:foo-2" {
		t.Fatalf("unarchive = %+v, want new_address proj:foo-2", got)
	}
	if e := r.active(t, "foo-2"); e.BranchName != "foo" {
		t.Errorf("branch_name = %q, want foo", e.BranchName)
	}
	if !r.isActive("foo") {
		t.Error("the active foo went missing")
	}
}

func TestUnarchive_RenamingBackToTheBranchNameClearsIt(t *testing.T) {
	r := start(t, "foo")
	if err := os.MkdirAll(filepath.Join(r.store, "proj", ".archive", "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.archive(t, false, false, "proj:foo") // archived as foo-2, branch_name foo

	got := r.unarchive(t, "proj:foo-2")[0]
	if got.Status != wire.TargetDone || got.NewAddress != "" {
		t.Fatalf("unarchive = %+v", got)
	}
	// foo-2 is free again, so the epic keeps that name and its branch_name stays foo.
	if e := r.active(t, "foo-2"); e.BranchName != "foo" {
		t.Errorf("branch_name = %q, want foo", e.BranchName)
	}
}

func TestUnarchive_UnknownTarget(t *testing.T) {
	r := start(t, "done")
	r.archive(t, false, false, "proj:done")
	res := r.unarchive(t, "proj:nope", "proj:done", "nowhere:x")
	if res[0].Reason != wire.ReasonCleanupUnknown || res[1].Status != wire.TargetDone || res[2].Reason != wire.ReasonCleanupUnknown {
		t.Errorf("results = %+v", res)
	}
	if got := r.unarchive(t, "proj:done")[0]; got.Reason != wire.ReasonCleanupUnknown {
		t.Errorf("an active epic = %+v, want unknown", got)
	}
}

func TestUnarchive_SessionsComeBackUnderTheRenamedRow(t *testing.T) {
	r := start(t, "foo")
	req := wire.CreateSessionRequest{Ticket: "proj:foo/01", FixOf: "proj:foo/01", Project: "proj", Dir: t.TempDir()}
	res, err := r.H.Client.Sessions.Create(r.Ctx, req)
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	if got := r.archive(t, true, false, "proj:foo")[0]; got.Status != wire.TargetDone {
		t.Fatalf("archive = %+v", got)
	}
	servertest.WriteCompleteEpic(t, r.store, "foo") // a new active foo

	if got := r.unarchive(t, "proj:foo")[0]; got.NewAddress != "proj:foo-2" {
		t.Fatalf("unarchive = %+v", got)
	}
	list, err := r.H.Client.Sessions.List(r.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.ID != id {
			continue
		}
		if want := "proj:foo-2/01"; s.Ticket != want || s.FixOf != want {
			t.Errorf("session keys = %q, %q, want %q", s.Ticket, s.FixOf, want)
		}
		return
	}
	t.Fatalf("session %s not listed", id)
}

func TestUnarchive_PairClashRenamesBothAndTheImplAddressResolves(t *testing.T) {
	r := startPair(t, "done", false)
	r.archive(t, false, false, "proj:foo")
	servertest.WriteCompleteEpic(t, r.store, "foo")

	got := r.unarchive(t, "proj:foo-impl")[0]
	if got.Status != wire.TargetDone || got.Target != "proj:foo-impl" || got.NewAddress != "proj:foo-2-impl" {
		t.Fatalf("unarchive = %+v", got)
	}
	if !r.isActive("foo-2") || !r.isActive("foo-2-impl") || len(r.archived(t)) != 0 {
		t.Errorf("pair not restored: archived = %v", r.archived(t))
	}
	if e := r.active(t, "foo-2"); e.BranchName != "foo" || !e.IsMap {
		t.Errorf("design = branch %q, map %v", e.BranchName, e.IsMap)
	}
	if e := r.active(t, "foo-2-impl"); !e.ArchivedAt.IsZero() {
		t.Error("the -impl half kept archived_at")
	}
}

func TestUnarchive_PublishesWorkspacesChanged(t *testing.T) {
	r := start(t, "done")
	r.archive(t, false, false, "proj:done")
	servertest.WaitForEvent(r.Ctx, t, r.Evs, wire.EventWorkspacesChanged, "")
	r.unarchive(t, "proj:done")
	servertest.WaitForEvent(r.Ctx, t, r.Evs, wire.EventWorkspacesChanged, "")
}
