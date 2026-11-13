package triage_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/triage"
	"github.com/elentok/gx/server/wire"
	triagerec "github.com/elentok/gx/triage"
)

func openTriage(t *testing.T, f rigs.DesignsFixture, req wire.TriageOpenRequest) wire.SessionResult {
	t.Helper()
	res, err := f.H.Client.Triage.Open(f.Ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// recordCount counts the live and past records of the project.
func recordCount(t *testing.T, s *triagerec.Store) int {
	t.Helper()
	past, err := s.History()
	if err != nil {
		t.Fatal(err)
	}
	if live, _ := s.LoadLive(); live != nil {
		return len(past) + 1
	}
	return len(past)
}

func TestOpen_CreatesRecordSnapshotAndOneSessionInTrunk(t *testing.T) {
	f, store := startViewFixture(t)
	var seen *triagerec.Record
	triage.BeforeLaunch = func(string) { seen, _ = store.LoadLive() }
	t.Cleanup(func() { triage.BeforeLaunch = nil })

	res := openTriage(t, f, wire.TriageOpenRequest{Project: "proj"})
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	if seen == nil || len(seen.Rows) != 2 {
		t.Fatalf("record at launch = %+v, want it with 2 snapshot rows", seen)
	}
	s := res.Session
	if s.Flow != "triage" || s.FlowKey != "proj" || s.Ticket != "" || s.Design {
		t.Fatalf("session = flow %q key %q ticket %q design %v", s.Flow, s.FlowKey, s.Ticket, s.Design)
	}
	meta := servertest.ReadMeta(t, f.H, s.ID)
	want, _ := filepath.EvalSymlinks(f.Repo)
	if got, _ := filepath.EvalSymlinks(meta["dir"].(string)); got != want {
		t.Fatalf("session dir = %q, want trunk checkout %q", got, want)
	}
	if live := f.Live(t); len(live) != 1 {
		t.Fatalf("live sessions = %d, want 1", len(live))
	}
}

func TestOpen_SecondOpenReturnsTheLiveSession(t *testing.T) {
	f, store := startViewFixture(t)
	first := openTriage(t, f, wire.TriageOpenRequest{Project: "proj"})
	rec, _ := store.LoadLive()
	second := openTriage(t, f, wire.TriageOpenRequest{Project: "proj"})
	if second.Session == nil || second.Session.ID != first.Session.ID {
		t.Fatalf("second = %+v, want session %s", second, first.Session.ID)
	}
	if live, _ := store.LoadLive(); live == nil || live.ID != rec.ID || recordCount(t, store) != 1 {
		t.Fatalf("live = %+v, records = %d, want only %s", live, recordCount(t, store), rec.ID)
	}
}

func TestOpen_ConcurrentOpensEndWithOneRecordAndSession(t *testing.T) {
	f, store := startViewFixture(t)
	ids := make([]string, 4)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Go(func() {
			if res, err := f.H.Client.Triage.Open(f.Ctx, wire.TriageOpenRequest{Project: "proj"}); err == nil && res.Session != nil {
				ids[i] = res.Session.ID
			}
		})
	}
	wg.Wait()
	for _, id := range ids {
		if id == "" || id != ids[0] {
			t.Fatalf("session ids = %v, want one id", ids)
		}
	}
	if n := recordCount(t, store); n != 1 {
		t.Fatalf("records = %d, want 1", n)
	}
	if live := f.Live(t); len(live) != 1 {
		t.Fatalf("live sessions = %d, want 1", len(live))
	}
}

func TestOpen_LiveRecordWithoutSessionLaunchesOnTheSameRecord(t *testing.T) {
	f, store := startViewFixture(t)
	rec := mustRec(store.Create())
	res := openTriage(t, f, wire.TriageOpenRequest{Project: "proj"})
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	if live, _ := store.LoadLive(); live == nil || live.ID != rec.ID || recordCount(t, store) != 1 {
		t.Fatalf("live = %+v, records = %d, want only %s", live, recordCount(t, store), rec.ID)
	}
}

func TestOpen_NewReplacesTheLiveSession(t *testing.T) {
	f, _ := startViewFixture(t)
	first := openTriage(t, f, wire.TriageOpenRequest{Project: "proj"}).Session
	servertest.WaitForHolder(t, filepath.Join(f.H.SessionsDir, first.ID))
	res := openTriage(t, f, wire.TriageOpenRequest{Project: "proj", New: true})
	if res.Refused || res.Session == nil || res.Session.ID == first.ID {
		t.Fatalf("new = %+v, want a fresh session", res)
	}
}

func TestOpen_WebNewNeedsAFreshToken(t *testing.T) {
	f, _ := startViewFixture(t)
	live := openTriage(t, f, wire.TriageOpenRequest{Project: "proj"}).Session
	servertest.WaitForHolder(t, filepath.Join(f.H.SessionsDir, live.ID))
	code, out := servertest.WebPostJSON(t, f.Base+"/v1/triage/open",
		fmt.Sprintf(`{"project":"proj","new":true,"state":%q}`, live.ID+":starting:1"), f.Ck, "", servertest.JSONPost(f.Base))
	if code != 200 || servertest.ReasonOf(t, out) != wire.ReasonStale {
		t.Fatalf("stale new = %d %s, want stale", code, out)
	}
	if got := f.Live(t); len(got) != 1 || got[0].ID != live.ID {
		t.Fatalf("live = %+v, want the original session kept", got)
	}
}

func TestOpen_RefusesAtApprove(t *testing.T) {
	f, store := startViewFixture(t)
	rec := mustRec(store.Create())
	rec.Step = "approve"
	if err := store.Save(rec); err != nil {
		t.Fatal(err)
	}
	res := openTriage(t, f, wire.TriageOpenRequest{Project: "proj"})
	if !res.Refused || res.Reason != wire.ReasonTriageApprove {
		t.Fatalf("open = %+v, want %s", res, wire.ReasonTriageApprove)
	}
	if live := f.Live(t); len(live) != 0 {
		t.Fatalf("live sessions = %d, want none", len(live))
	}
}

func TestOpen_RefusesWithNoOpenFollowUp(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "follow-ups", "01", "done one", servertest.TicketOpts{Status: "done"})
	f := rigs.StartDesignsIn(t, store)
	res := openTriage(t, f, wire.TriageOpenRequest{Project: "proj"})
	if !res.Refused || res.Reason != wire.ReasonTriageNoFollowUps {
		t.Fatalf("open = %+v, want %s", res, wire.ReasonTriageNoFollowUps)
	}
	if rec, _ := triagerec.NewStore(filepath.Join(store, "proj")).LoadLive(); rec != nil {
		t.Fatalf("a record was created: %+v", rec)
	}
}
