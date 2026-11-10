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
