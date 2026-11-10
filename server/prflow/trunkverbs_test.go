package prflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// trunkBlockedRig is a squash-merged PR whose Trunk update is blocked: the person's
// uncommitted epic-a.txt clashes with the merge. The web verbs are on.
func trunkBlockedRig(t *testing.T) (f *rigs.PRFlow, clash, sha string) {
	t.Helper()
	f = rigs.StartPRFlow(t, rigs.PRFlowOpts{Server: []func(*server.Config){func(c *server.Config) { c.WebAddr = "127.0.0.1:0" }}})
	servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "01", "root", servertest.TicketOpts{Status: "done"})
	servertest.CommitOnBranch(t, f.Origin.Local, "epic-a")
	openPR(f)
	f.StampPR(t, 5, prURL)
	sha = f.Origin.SimulateSquashMerge("epic-a")
	clash = filepath.Join(f.Origin.Local, "epic-a.txt")
	if err := os.WriteFile(clash, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.Tick(t)
	if k := tickets.LoadEpicAt(f.EpicDir()).MergeBlockKind; k != string(schema.PausePRDirtyTrunk) {
		t.Fatalf("block kind = %q, want pr-dirty-trunk", k)
	}
	return f, clash, sha
}

func postEpicVerb(t *testing.T, f *rigs.PRFlow, verb string) string {
	t.Helper()
	hs, err := f.H.Client.Handshake(t.Context())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	cookie := servertest.Login(t, f.H.StateDir, hs.WebURL)
	body := `{"address":"proj:epic-a","state":"` + string(wire.EpicListMergeBlocked) + `"}`
	code, out := servertest.WebPostJSON(t, hs.WebURL+"/v1/epics/"+verb, body, cookie, "", servertest.JSONPost(hs.WebURL))
	if code != 200 || servertest.ReasonOf(t, out) != "" {
		t.Fatalf("%s = %d %s", verb, code, out)
	}
	return out
}

func TestTrunkVerbs_MergeAgainKeepsTheBlockWhileDirtyThenStamps(t *testing.T) {
	f, clash, sha := trunkBlockedRig(t)

	postEpicVerb(t, f, "merge")
	if e := tickets.LoadEpicAt(f.EpicDir()); !e.MergedAt.IsZero() || e.MergeBlockKind != string(schema.PausePRDirtyTrunk) {
		t.Fatalf("still dirty: merged_at %v, block %q", e.MergedAt, e.MergeBlockKind)
	}
	if err := os.Remove(clash); err != nil {
		t.Fatal(err)
	}
	postEpicVerb(t, f, "merge")

	if e := tickets.LoadEpicAt(f.EpicDir()); e.MergedAt.IsZero() || e.MergeBlockKind != "" {
		t.Errorf("after cleaning: merged_at %v, block %q", e.MergedAt, e.MergeBlockKind)
	}
	if got := strings.TrimSpace(servertest.GitOut(t, f.Origin.Local, "rev-parse", "main")); got != sha {
		t.Errorf("local main = %s, want %s", got, sha)
	}
	if ks := kinds(t, f); count(ks, prflow.KindTrunkUpdated) != 1 {
		t.Errorf("activity = %v, want one trunk-updated", ks)
	}
}

func TestTrunkVerbs_StashMergeRestoresThePersonsFiles(t *testing.T) {
	f, clash, sha := trunkBlockedRig(t)

	postEpicVerb(t, f, "stash-merge")

	if e := tickets.LoadEpicAt(f.EpicDir()); e.MergedAt.IsZero() || e.MergeBlockKind != "" {
		t.Errorf("merged_at %v, block %q", e.MergedAt, e.MergeBlockKind)
	}
	if got := strings.TrimSpace(servertest.GitOut(t, f.Origin.Local, "rev-parse", "main")); got != sha {
		t.Errorf("local main = %s, want %s", got, sha)
	}
	raw, err := os.ReadFile(clash)
	if err != nil || !strings.Contains(string(raw), "mine") {
		t.Errorf("epic-a.txt = %q (err %v), want the person's content back", raw, err)
	}
}

func TestTrunkVerbs_MergeAgainRetriesAFailedTrunkUpdate(t *testing.T) {
	f, clash, sha := trunkBlockedRig(t)
	if err := os.Remove(clash); err != nil {
		t.Fatal(err)
	}
	servertest.RunGitIn(t, f.Origin.Local, "commit", "-q", "--allow-empty", "-m", "local only")
	postEpicVerb(t, f, "merge")
	if k := tickets.LoadEpicAt(f.EpicDir()).MergeBlockKind; k != string(schema.PausePRTrunkUpdate) {
		t.Fatalf("block kind = %q, want pr-trunk-update", k)
	}
	servertest.RunGitIn(t, f.Origin.Local, "reset", "-q", "--hard", "HEAD~1")

	postEpicVerb(t, f, "merge")

	if e := tickets.LoadEpicAt(f.EpicDir()); e.MergedAt.IsZero() || e.MergeBlockKind != "" {
		t.Errorf("merged_at %v, block %q", e.MergedAt, e.MergeBlockKind)
	}
	if got := strings.TrimSpace(servertest.GitOut(t, f.Origin.Local, "rev-parse", "main")); got != sha {
		t.Errorf("local main = %s, want %s", got, sha)
	}
}
