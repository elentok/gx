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

// rig is a server over a store whose proj has the given done epics.
type rig struct {
	servertest.DesignsFixture
	store string
}

func start(t *testing.T, doneEpics ...string) rig {
	t.Helper()
	store := t.TempDir()
	for _, e := range doneEpics {
		servertest.WriteCompleteEpic(t, store, e)
	}
	return rig{servertest.StartDesignsIn(t, store), store}
}

func (r rig) archive(t *testing.T, yes, dryRun bool, targets ...string) []wire.TargetResult {
	t.Helper()
	res, err := r.H.Client.Admin.ArchiveEpics(r.Ctx, targets, yes, dryRun)
	if err != nil {
		t.Fatal(err)
	}
	return res.Results
}

func (r rig) archived(t *testing.T) map[string]bool {
	t.Helper()
	names, err := tickets.ArchivedEpicNames(filepath.Join(r.store, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func (r rig) epic(t *testing.T, name string) tickets.Epic {
	t.Helper()
	epics, err := tickets.LoadArchived(filepath.Join(r.store, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range epics {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no archived epic %s", name)
	return tickets.Epic{}
}

func TestArchive_DoneEpicMovesIntoTheArchiveAndIsStamped(t *testing.T) {
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

	res := r.archive(t, false, false, "proj:done")
	if len(res) != 1 || res[0].Status != wire.TargetDone || res[0].NewAddress != "" {
		t.Fatalf("results = %+v", res)
	}
	if !r.archived(t)["done"] {
		t.Fatal("epic not in .archive")
	}
	e := r.epic(t, "done")
	if e.ArchivedAt.IsZero() || e.ArchivedWorkspace != "Bugs" || e.BranchName != "" {
		t.Errorf("stamps = %v %q %q, want archived_at, Bugs and no branch_name", e.ArchivedAt, e.ArchivedWorkspace, e.BranchName)
	}
	snap, err := r.H.Client.Snapshot(r.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(snap.Held, "proj:done") {
		t.Errorf("hold mark kept: %v", snap.Held)
	}
}

func TestArchive_NotOnTrunkArchivesAndTheDryRunSaysSo(t *testing.T) {
	r := start(t, "unlanded")

	dry := r.archive(t, false, true, "proj:unlanded")[0]
	if dry.Status != wire.TargetFits || dry.OnTrunk == nil || *dry.OnTrunk {
		t.Fatalf("dry run = %+v, want fits with on_trunk false", dry)
	}
	if len(r.archived(t)) != 0 {
		t.Fatal("dry run moved the folder")
	}
	if got := r.archive(t, false, false, "proj:unlanded")[0]; got.Status != wire.TargetDone {
		t.Fatalf("archive = %+v", got)
	}
	if !r.archived(t)["unlanded"] {
		t.Error("epic not archived")
	}
}

func TestArchive_RefusalsAndManyTargets(t *testing.T) {
	r := start(t, "done")
	servertest.WriteTicket(t, r.store, "proj", "open", "01", "first", "")

	res := r.archive(t, false, false, "proj:open", "proj:nope", "proj:done")
	want := []struct{ status, reason string }{
		{wire.TargetRefused, wire.ReasonCleanupNotDone},
		{wire.TargetRefused, wire.ReasonCleanupUnknown},
		{wire.TargetDone, ""},
	}
	for i, w := range want {
		if res[i].Status != w.status || res[i].Reason != w.reason {
			t.Errorf("result %d = %+v, want %+v", i, res[i], w)
		}
	}
	if !r.archived(t)["done"] || r.archived(t)["open"] {
		t.Errorf("archived = %v", r.archived(t))
	}
}

func TestArchive_DesignTargetIsNotFinished(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.WriteTicket(t, store, "proj", "map-impl", "01", "first", "")
	r := rig{servertest.StartDesignsIn(t, store), store}

	for _, addr := range []string{"proj:map", "proj:map-impl"} {
		got := r.archive(t, true, false, addr)[0]
		if got.Reason != wire.ReasonArchiveNotFinished {
			t.Errorf("%s = %+v, want not-finished", addr, got)
		}
	}
}

func TestArchive_ClashTakesTheNextSuffixAndKeepsTheBranchName(t *testing.T) {
	r := start(t, "foo")
	if err := os.MkdirAll(filepath.Join(r.store, "proj", ".archive", "foo"), 0o755); err != nil {
		t.Fatal(err)
	}

	dry := r.archive(t, false, true, "proj:foo")[0]
	if dry.NewAddress != "proj:foo-2" {
		t.Fatalf("dry run = %+v, want new_address proj:foo-2", dry)
	}
	got := r.archive(t, false, false, "proj:foo")[0]
	if got.Status != wire.TargetDone || got.NewAddress != "proj:foo-2" {
		t.Fatalf("archive = %+v", got)
	}
	if e := r.epic(t, "foo-2"); e.BranchName != "foo" {
		t.Errorf("branch_name = %q, want foo", e.BranchName)
	}
}

func TestArchive_LiveSessionsNeedYesThenStopAndAreRekeyed(t *testing.T) {
	r := start(t, "done")
	create := func(req wire.CreateSessionRequest) string {
		t.Helper()
		req.Project, req.Dir = "proj", t.TempDir()
		res, err := r.H.Client.Sessions.Create(r.Ctx, req)
		if err != nil || res.Session == nil {
			t.Fatalf("create %+v = %+v, %v", req, res, err)
		}
		return res.Session.ID
	}
	fix := create(wire.CreateSessionRequest{Ticket: "proj:done/01", FixOf: "proj:done/01"})

	got := r.archive(t, false, false, "proj:done")[0]
	if got.Reason != wire.ReasonNeedsYes || !slices.Equal(got.Sessions, []string{fix}) {
		t.Fatalf("without yes = %+v, want needs-yes naming %s", got, fix)
	}
	if len(r.archived(t)) != 0 {
		t.Fatal("a refused row moved the folder")
	}
	if dry := r.archive(t, false, true, "proj:done")[0]; dry.Status != wire.TargetFits || !slices.Equal(dry.Sessions, []string{fix}) {
		t.Errorf("dry run = %+v, want fits listing %s", dry, fix)
	}

	got = r.archive(t, true, false, "proj:done")[0]
	if got.Status != wire.TargetDone || !slices.Equal(got.Sessions, []string{fix}) {
		t.Fatalf("with yes = %+v", got)
	}
	list, err := r.H.Client.Sessions.List(r.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.ID != fix {
			continue
		}
		if s.Status.State != wire.SessionExited {
			t.Errorf("session state = %s, want exited", s.Status.State)
		}
		if want := "proj:.archive/done/01"; s.Ticket != want || s.FixOf != want {
			t.Errorf("session keys = %q, %q, want %q", s.Ticket, s.FixOf, want)
		}
		return
	}
	t.Fatalf("session %s not listed", fix)
}

// stoppedFixSession is a stopped Session of ticket.
func (r rig) stoppedFixSession(t *testing.T, ticket string) string {
	t.Helper()
	res, err := r.H.Client.Sessions.Create(r.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Ticket: ticket, FixOf: ticket})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	if _, err := r.H.Client.Sessions.Stop(r.Ctx, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// lock makes the Session's files unwritable, so a re-key of it fails.
func (r rig) lock(t *testing.T, id string) {
	t.Helper()
	dir := filepath.Join(r.H.SessionsDir, id)
	meta := filepath.Join(dir, "meta.json")
	for path, mode := range map[string]os.FileMode{meta: 0o444, dir: 0o555} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
	}
}

func TestArchive_FailedSessionRekeyIsAFailedTarget(t *testing.T) {
	r := start(t, "done")
	r.lock(t, r.stoppedFixSession(t, "proj:done/01"))

	got := r.archive(t, true, false, "proj:done")[0]
	if got.Status != wire.TargetFailed || !strings.Contains(got.Message, "re-key session") {
		t.Fatalf("archive = %+v, want a failed target naming the re-key", got)
	}
}

func TestArchive_PublishesWorkspacesChanged(t *testing.T) {
	r := start(t, "done")
	r.archive(t, false, false, "proj:done")
	servertest.WaitForEvent(r.Ctx, t, r.Evs, wire.EventWorkspacesChanged, "")
}
