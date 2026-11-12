package archive_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/workspaces"
	"github.com/elentok/gx/testutil"
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

// workspaceOf is the workspace the Epics list shows for the epic card at address.
// It waits for the row: the store rescan after a move is asynchronous.
func (r rig) workspaceOf(t *testing.T, address string) string {
	t.Helper()
	var workspace string
	testutil.WaitFor(t, 5*time.Second, "epic row "+address, func() bool {
		list, err := r.H.Client.Reads.Epics(r.Ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range list.Epics {
			if e.Address == address {
				workspace = e.Workspace
				return true
			}
		}
		return false
	})
	return workspace
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
	if got := r.workspaceOf(t, "proj:done"); got != workspaces.DefaultName {
		t.Errorf("workspace = %q, want %q", got, workspaces.DefaultName)
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

// placedUnder lists the ids of the sessions the snapshot places under epic, and
// the ids it places under Other.
func (r rig) placedUnder(t *testing.T, epic string) (under, other []string) {
	t.Helper()
	snap, err := r.H.Client.Snapshot(r.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range snap.Placements {
		switch {
		case p.Kind == wire.PlacementOther:
			other = append(other, p.Session)
		case p.Kind == wire.PlacementEpic && p.Epic == epic:
			under = append(under, p.Session)
		}
	}
	slices.Sort(under)
	return under, other
}

func TestArchiveUnarchive_FixAndDesignSessionsLeaveOtherEmptyAndComeBackUnderTheirRows(t *testing.T) {
	r := startPair(t, "done", false)
	fix := r.stoppedFixSession(t, "proj:foo-impl/01")
	design := r.Open(t, "proj:foo", false)
	if design.Refused || design.Session == nil {
		t.Fatalf("open = %+v", design)
	}
	ids := []string{fix, design.Session.ID}
	slices.Sort(ids)

	if got := r.archive(t, true, false, "proj:foo")[0]; got.Status != wire.TargetDone {
		t.Fatalf("archive = %+v", got)
	}
	servertest.WriteCompleteEpic(t, r.store, "foo") // a new active foo
	list, err := r.H.Client.Sessions.List(r.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if slices.Contains(ids, s.ID) && s.Status.State != wire.SessionExited {
			t.Errorf("session %s state = %s, want exited by the Archive", s.ID, s.Status.State)
		}
	}
	if under, other := r.placedUnder(t, "proj:foo"); len(under) != 0 || len(other) != 0 {
		t.Errorf("while archived: under the new foo = %v, Other = %v, want both empty", under, other)
	}

	if got := r.unarchive(t, "proj:foo")[0]; got.NewAddress != "proj:foo-2" {
		t.Fatalf("unarchive = %+v", got)
	}
	// The -impl half's Fix Session joins the Design row once the index sees foo-2.
	servertest.WaitUntil(t, "sessions placed under foo-2", func() bool {
		under, _ := r.placedUnder(t, "proj:foo-2")
		return slices.Equal(under, ids)
	})
	under, other := r.placedUnder(t, "proj:foo-2")
	if !slices.Equal(under, ids) || len(other) != 0 {
		t.Errorf("after Unarchive: under foo-2 = %v, Other = %v, want %v and none", under, other, ids)
	}
	if old, _ := r.placedUnder(t, "proj:foo"); len(old) != 0 {
		t.Errorf("the new foo shows the old Sessions %v", old)
	}
	list, err = r.H.Client.Sessions.List(r.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if !slices.Contains(ids, s.ID) {
			continue
		}
		if !strings.HasPrefix(s.Ticket, "proj:foo-2") || (s.FixOf != "" && !strings.HasPrefix(s.FixOf, "proj:foo-2")) {
			t.Errorf("session %s keys = %q, %q, want both under proj:foo-2", s.ID, s.Ticket, s.FixOf)
		}
	}
}

// unarchivedAsClashedDesign archives the foo pair with a kept design worktree
// that holds one commit, then Unarchives it as foo-2 around a new active foo.
// It returns the worktree path.
func (r rig) unarchivedAsClashedDesign(t *testing.T) string {
	t.Helper()
	wt := r.withDesignWorktree(t, true)
	testutil.WriteFile(t, wt, "design.txt", "design")
	testutil.CommitAll(t, wt, "design work")
	if got := r.archive(t, false, false, "proj:foo")[0]; got.Status != wire.TargetDone {
		t.Fatalf("archive = %+v", got)
	}
	servertest.WriteCompleteEpic(t, r.store, "foo")
	if got := r.unarchive(t, "proj:foo")[0]; got.NewAddress != "proj:foo-2" {
		t.Fatalf("unarchive = %+v, want new_address proj:foo-2", got)
	}
	return wt
}

func TestUnarchive_ClashedDesignCleanupFollowsBranchNameAndChecksTheImplBranch(t *testing.T) {
	cases := []struct {
		name       string
		implAtTip  bool
		wantKept   bool
		wantReason string
	}{
		{"design patches all in foo-impl", true, false, "worktree and branch removed"},
		{"a design patch missing from foo-impl", false, true, "worktree and branch kept"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := startPair(t, "done", false)
			wt := r.unarchivedAsClashedDesign(t)
			target := "main"
			if c.implAtTip {
				target = "foo-design"
			}
			servertest.RunGitIn(t, r.Repo, "branch", "-f", "foo-impl", target)

			if res := r.Open(t, "proj:foo-2", false); res.Refused {
				t.Fatalf("open = %+v", res)
			}
			_, statErr := os.Stat(wt)
			if kept := statErr == nil; kept != c.wantKept {
				t.Errorf("design worktree kept = %v, want %v (%s)", kept, c.wantKept, c.wantReason)
			}
			if kept := r.branchExists(t, "foo-design"); kept != c.wantKept {
				t.Errorf("design branch kept = %v, want %v (%s)", kept, c.wantKept, c.wantReason)
			}
		})
	}
}

func TestUnarchive_PublishesWorkspacesChanged(t *testing.T) {
	r := start(t, "done")
	r.archive(t, false, false, "proj:done")
	servertest.WaitForEvent(r.Ctx, t, r.Evs, wire.EventWorkspacesChanged, "")
	r.unarchive(t, "proj:done")
	servertest.WaitForEvent(r.Ctx, t, r.Evs, wire.EventWorkspacesChanged, "")
}
