package serverapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func archiveFixture(t *testing.T, done ...string) (servertest.DesignsFixture, string) {
	t.Helper()
	store := t.TempDir()
	for _, e := range done {
		servertest.WriteCompleteEpic(t, store, e)
	}
	return servertest.StartDesignsIn(t, store), store
}

func archivedNames(t *testing.T, store string) map[string]bool {
	t.Helper()
	names, err := tickets.ArchivedEpicNames(filepath.Join(store, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func TestServerCleanupWrite_ArchiveManyTargetsExitsOneOnARefusal(t *testing.T) {
	f, store := archiveFixture(t, "one", "two")

	out, err := runServer(t, f.H, "epics", "archive", "proj:one", "proj:nope", "proj:two")
	if exit, ok := err.(*cmdbase.ExitError); !ok || exit.Code != 1 {
		t.Fatalf("err = %v, want exit 1", err)
	}
	for _, want := range []string{"done proj:one", "refused proj:nope", "done proj:two", "2 done, 1 refused"} {
		if !strings.Contains(out, want) {
			t.Errorf("out = %q, missing %q", out, want)
		}
	}
	if got := archivedNames(t, store); !got["one"] || !got["two"] {
		t.Errorf("archived = %v", got)
	}
}

func TestServerCleanupWrite_ArchiveJSONIsAnEnvelopeForOneTarget(t *testing.T) {
	f, _ := archiveFixture(t, "one")

	out, err := runServer(t, f.H, "epics", "archive", "proj:one", "--dry-run", "--json")
	var res wire.TargetsResult
	if err != nil || json.Unmarshal([]byte(out), &res) != nil || len(res.Results) != 1 {
		t.Fatalf("out = %q, err = %v, want a results envelope", out, err)
	}
	if r := res.Results[0]; r.Status != wire.TargetFits || r.OnTrunk == nil || *r.OnTrunk {
		t.Errorf("result = %+v, want fits and on_trunk false", r)
	}
}

func TestServerCleanupWrite_ArchiveDesignTargetsJSON(t *testing.T) {
	store := t.TempDir()
	design := func(name, step string, withImpl bool) {
		servertest.WriteCompleteEpic(t, store, name)
		md := "---\nkind: map\ndesign_step: " + step + "\n---\n"
		if err := os.WriteFile(filepath.Join(store, "proj", name, "ticket.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		if withImpl {
			servertest.WriteCompleteEpic(t, store, name+"-impl")
		}
	}
	design("foo", "done", true)
	design("bar", "chart", true)
	design("baz", "done", true)
	f := servertest.StartDesignsIn(t, store)

	out, err := runServer(t, f.H, "epics", "archive", "proj:foo", "proj:bar", "proj:baz-impl", "--json")
	if exit, ok := err.(*cmdbase.ExitError); !ok || exit.Code != 1 {
		t.Fatalf("err = %v, want exit 1 for the refused Design", err)
	}
	var res wire.TargetsResult
	if json.Unmarshal([]byte(out), &res) != nil || len(res.Results) != 3 {
		t.Fatalf("out = %q, want 3 results", out)
	}
	want := []struct{ target, status, reason string }{
		{"proj:foo", wire.TargetDone, ""},
		{"proj:bar", wire.TargetRefused, wire.ReasonArchiveNotFinished},
		{"proj:baz-impl", wire.TargetDone, ""},
	}
	for i, w := range want {
		if r := res.Results[i]; r.Target != w.target || r.Status != w.status || r.Reason != w.reason {
			t.Errorf("result %d = %+v, want %+v", i, r, w)
		}
	}
	got := archivedNames(t, store)
	if !got["foo"] || !got["foo-impl"] || !got["baz"] || !got["baz-impl"] || got["bar"] {
		t.Errorf("archived = %v", got)
	}
}

func TestServerCleanupWrite_ArchiveNeedsYesForLiveSessions(t *testing.T) {
	f, store := archiveFixture(t, "one")
	res, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Ticket: "proj:one/01", FixOf: "proj:one/01"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID

	if _, err := runServer(t, f.H, "epics", "archive", "proj:one"); err == nil || !strings.Contains(err.Error(), wire.ReasonNeedsYes) {
		t.Fatalf("without --yes err = %v, want needs-yes", err)
	}
	if len(archivedNames(t, store)) != 0 {
		t.Fatal("a refused archive moved the folder")
	}
	out, err := runServer(t, f.H, "epics", "archive", "proj:one", "--yes")
	if err != nil || !strings.Contains(out, id) {
		t.Fatalf("--yes = %q, %v, want the stopped session listed", out, err)
	}
}

func TestServerCleanupWrite_CleanupArchiveAliasTakesYes(t *testing.T) {
	f, store := archiveFixture(t, "one")
	res, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Ticket: "proj:one/01", FixOf: "proj:one/01"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	if _, err := runServer(t, f.H, "cleanup-archive", "--project", "proj", "--epic", "one"); err == nil {
		t.Fatal("alias without --yes archived an epic with a live session")
	}
	if _, err := runServer(t, f.H, "cleanup-archive", "--project", "proj", "--epic", "one", "--yes"); err != nil {
		t.Fatal(err)
	}
	if !archivedNames(t, store)["one"] {
		t.Error("alias did not archive the epic")
	}
}

func TestServerEpicsArchived_JSONListsNewestFirst(t *testing.T) {
	f, _ := archiveFixture(t, "first", "second")
	// Archived one after the other, so "second" has the newer archived_at.
	for _, epic := range []string{"proj:first", "proj:second"} {
		if _, err := runServer(t, f.H, "epics", "archive", epic); err != nil {
			t.Fatal(err)
		}
	}

	out, err := runServer(t, f.H, "epics", "archived", "--json")
	var list wire.ArchivedEpicList
	if err != nil || json.Unmarshal([]byte(out), &list) != nil {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if len(list.Epics) != 2 || list.Epics[0].Address != "proj:second" || list.Epics[1].Address != "proj:first" {
		t.Errorf("epics = %+v, want second then first", list.Epics)
	}
}

func TestServerEpicsArchivedDetail_JSONHasArchivedAt(t *testing.T) {
	f, _ := archiveFixture(t, "foo")
	if _, err := runServer(t, f.H, "epics", "archive", "proj:foo"); err != nil {
		t.Fatal(err)
	}

	out, err := runServer(t, f.H, "epics", "archived-detail", "proj:foo", "--json")
	var d wire.EpicDetail
	if err != nil || json.Unmarshal([]byte(out), &d) != nil {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if d.Address != "proj:foo" || d.ArchivedAt.IsZero() {
		t.Errorf("detail = %+v, want proj:foo with archived_at", d)
	}
}

func TestServerEpicsArchivedDetail_UnknownAddressFails(t *testing.T) {
	f, _ := archiveFixture(t, "foo")
	if out, err := runServer(t, f.H, "epics", "archived-detail", "proj:foo"); err == nil {
		t.Errorf("out = %q, want an error for an epic that is not archived", out)
	}
}

func TestServerEpicsUnarchive_ManyTargetsExitsOneOnUnknown(t *testing.T) {
	f, store := archiveFixture(t, "one", "two")
	if _, err := runServer(t, f.H, "epics", "archive", "proj:one", "proj:two"); err != nil {
		t.Fatal(err)
	}

	out, err := runServer(t, f.H, "epics", "unarchive", "proj:one", "proj:nope", "proj:two")
	if exit, ok := err.(*cmdbase.ExitError); !ok || exit.Code != 1 {
		t.Fatalf("err = %v, want exit 1", err)
	}
	for _, want := range []string{"done proj:one", "refused proj:nope", "done proj:two", "2 done, 1 refused"} {
		if !strings.Contains(out, want) {
			t.Errorf("out = %q, missing %q", out, want)
		}
	}
	if got := archivedNames(t, store); len(got) != 0 {
		t.Errorf("archived = %v, want none", got)
	}
}

func TestServerEpicsUnarchive_JSONIsAnEnvelopeForOneTargetWithNewAddress(t *testing.T) {
	f, store := archiveFixture(t, "foo")
	if _, err := runServer(t, f.H, "epics", "archive", "proj:foo"); err != nil {
		t.Fatal(err)
	}
	servertest.WriteCompleteEpic(t, store, "foo")

	out, err := runServer(t, f.H, "epics", "unarchive", "proj:foo", "--json")
	var res wire.TargetsResult
	if err != nil || json.Unmarshal([]byte(out), &res) != nil || len(res.Results) != 1 {
		t.Fatalf("out = %q, err = %v, want a results envelope", out, err)
	}
	if r := res.Results[0]; r.Status != wire.TargetDone || r.NewAddress != "proj:foo-2" {
		t.Errorf("result = %+v, want done with new_address proj:foo-2", r)
	}
}
