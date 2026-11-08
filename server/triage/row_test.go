package triage_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	triagerec "github.com/elentok/gx/triage"
)

// toApprove plans the two fixture rows (first ready, second merge into first)
// and returns the record at step approve plus the two addresses.
func toApprove(t *testing.T, store *triagerec.Store) (*triagerec.Record, string, string) {
	t.Helper()
	rec := mustRec(store.Create())
	a, b := rec.Rows[0].Address, rec.Rows[1].Address
	for _, addr := range []string{a, b} {
		rec = mustRec(store.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addr}))
	}
	rec = mustRec(store.SavePlan(rec.ID, rec.Revision, []triagerec.PlanRow{
		{RowID: a, Action: "ready"},
		{RowID: b, Action: "merge", Destination: a},
	}, nil))
	if rec.Step != "approve" {
		t.Fatalf("step = %s, want approve", rec.Step)
	}
	return rec, a, b
}

func saveRow(t *testing.T, f servertest.DesignsFixture, req wire.TriageRowRequest) wire.TriageEditResult {
	t.Helper()
	req.Project = "proj"
	res, err := f.H.Client.Triage.SaveRow(f.Ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func rowOf(v wire.TriageView, addr string) wire.TriageRow {
	for _, r := range v.Rows {
		if r.Address == addr {
			return r
		}
	}
	return wire.TriageRow{}
}

func TestRow_EditSavesAndTheViewShowsItChanged(t *testing.T) {
	f, store := startViewFixture(t)
	rec, a, _ := toApprove(t, store)
	res := saveRow(t, f, wire.TriageRowRequest{
		Revision: rec.Revision, Address: a, Action: "map", Destination: "new-epic", Answers: map[string]string{"q1": "yes"},
	})
	if res.Refused || res.Revision != rec.Revision+1 {
		t.Fatalf("save = %+v", res)
	}
	v := show(t, f)
	row := rowOf(v, a)
	if row.Action != "map" || row.Destination != "new-epic" || !row.PersonChanged || row.Answers["q1"] != "yes" || v.ChangedRows != 1 {
		t.Fatalf("row = %+v, changed rows = %d", row, v.ChangedRows)
	}
}

func TestRow_KeepForLaterSaves(t *testing.T) {
	f, store := startViewFixture(t)
	rec, a, _ := toApprove(t, store)
	if res := saveRow(t, f, wire.TriageRowRequest{Revision: rec.Revision, Address: a, KeepForLater: true}); res.Refused {
		t.Fatalf("save = %+v", res)
	}
	if got := rowOf(show(t, f), a).Action; got != "keep" {
		t.Fatalf("action = %q, want keep", got)
	}
}

func TestRow_SecondSaveOnOneRevisionIsRefused(t *testing.T) {
	f, store := startViewFixture(t)
	rec, a, b := toApprove(t, store)
	if res := saveRow(t, f, wire.TriageRowRequest{Revision: rec.Revision, Address: a, Action: "map", Destination: "e"}); res.Refused {
		t.Fatalf("first save = %+v", res)
	}
	res := saveRow(t, f, wire.TriageRowRequest{Revision: rec.Revision, Address: b, Action: "close", Destination: "dup"})
	if !res.Refused || res.Reason != wire.ReasonStale {
		t.Fatalf("second save = %+v, want stale", res)
	}
	if rowOf(show(t, f), b).PersonChanged {
		t.Fatal("the refused save was stored")
	}
}

func TestRow_MergeIntoAcceptsOnlyReadyAndMapRows(t *testing.T) {
	f, store := startViewFixture(t)
	rec, a, b := toApprove(t, store)
	// a is ready: merging b into it is accepted.
	res := saveRow(t, f, wire.TriageRowRequest{Revision: rec.Revision, Address: b, Action: "merge", Destination: a})
	if res.Refused {
		t.Fatalf("merge into ready = %+v", res)
	}
	// a becomes close: a is no longer a target, and a cannot merge into b either.
	res = saveRow(t, f, wire.TriageRowRequest{Revision: res.Revision, Address: a, Action: "merge", Destination: b})
	if !res.Refused || res.Reason != wire.ReasonTriagePhase {
		t.Fatalf("merge into a merge row = %+v, want refused", res)
	}
}

func TestRow_RepointingATargetFlagsItsMergedRowsAndApproveWaits(t *testing.T) {
	f, store := startViewFixture(t)
	rec, a, b := toApprove(t, store)
	res := saveRow(t, f, wire.TriageRowRequest{Revision: rec.Revision, Address: a, Action: "close", Destination: "dup"})
	if res.Refused {
		t.Fatalf("save = %+v", res)
	}
	v := show(t, f)
	if !rowOf(v, b).MergeOrphan || rowOf(v, a).MergeOrphan {
		t.Fatalf("orphan flags: b=%v a=%v, want b only", rowOf(v, b).MergeOrphan, rowOf(v, a).MergeOrphan)
	}
	if _, err := store.Approve(rec.ID, res.Revision); err == nil {
		t.Fatal("approve succeeded with an orphaned merge")
	}
	res = saveRow(t, f, wire.TriageRowRequest{Revision: res.Revision, Address: b, Action: "close", Destination: "dup"})
	if res.Refused {
		t.Fatalf("re-point = %+v", res)
	}
	if rowOf(show(t, f), b).MergeOrphan {
		t.Fatal("b still flagged after re-pointing")
	}
	if _, err := store.Approve(rec.ID, res.Revision); err != nil {
		t.Fatalf("approve after re-pointing: %v", err)
	}
}

func TestRow_RefusedAtVerifyAndPlan(t *testing.T) {
	f, store := startViewFixture(t)
	rec := mustRec(store.Create())
	a := rec.Rows[0].Address
	res := saveRow(t, f, wire.TriageRowRequest{Revision: rec.Revision, Address: a, Action: "close"})
	if !res.Refused || res.Reason != wire.ReasonTriagePhase {
		t.Fatalf("at verify = %+v", res)
	}
	for _, r := range rec.Rows {
		rec = mustRec(store.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: r.Address}))
	}
	res = saveRow(t, f, wire.TriageRowRequest{Revision: rec.Revision, Address: a, Action: "close"})
	if !res.Refused || res.Reason != wire.ReasonTriagePhase {
		t.Fatalf("at plan = %+v", res)
	}
}

// ticketFiles lists every file under the store with its content.
func ticketFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".md" {
			return err
		}
		b, err := os.ReadFile(p)
		out[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAbandon_AtPlanEndsSessionAndMovesToHistoryWithoutTouchingTickets(t *testing.T) {
	root := t.TempDir()
	servertest.WriteTicketWith(t, root, "proj", "follow-ups", "01", "first", servertest.TicketOpts{})
	servertest.WriteTicketWith(t, root, "proj", "follow-ups", "02", "second", servertest.TicketOpts{})
	f := servertest.StartDesignsIn(t, root)
	store := triagerec.NewStore(filepath.Join(root, "proj"))
	sess := openTriage(t, f, wire.TriageOpenRequest{Project: "proj"}).Session
	servertest.WaitForHolder(t, filepath.Join(f.H.SessionsDir, sess.ID))
	rec, _ := store.LoadLive()
	for _, r := range rec.Rows {
		rec = mustRec(store.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: r.Address}))
	}
	before := ticketFiles(t, root)

	res, err := f.H.Client.Triage.Abandon(f.Ctx, wire.TriageAbandonRequest{Project: "proj", Revision: rec.Revision})
	if err != nil || res.Refused {
		t.Fatalf("abandon = %+v, %v", res, err)
	}
	if live, _ := store.LoadLive(); live != nil {
		t.Fatalf("record still live: %+v", live)
	}
	v := show(t, f)
	if v.State != wire.TriageAbandoned || len(v.History) != 1 || v.History[0].State != wire.TriageAbandoned {
		t.Fatalf("view = state %s, history %+v", v.State, v.History)
	}
	servertest.WaitFor(t, 10*time.Second, "the Triage session to stop", func() bool { return len(f.Live(t)) == 0 })
	after := ticketFiles(t, root)
	if len(after) != len(before) {
		t.Fatalf("ticket files %d -> %d", len(before), len(after))
	}
	for p, c := range before {
		if after[p] != c {
			t.Fatalf("ticket %s changed", p)
		}
	}
}

func TestAbandon_RefusedAtWriteAndWhenStale(t *testing.T) {
	f, store := startViewFixture(t)
	rec, _, _ := toApprove(t, store)
	res, err := f.H.Client.Triage.Abandon(f.Ctx, wire.TriageAbandonRequest{Project: "proj", Revision: rec.Revision + 5})
	if err != nil || !res.Refused || res.Reason != wire.ReasonStale {
		t.Fatalf("stale abandon = %+v, %v", res, err)
	}
	rec = mustRec(store.Approve(rec.ID, rec.Revision))
	res, err = f.H.Client.Triage.Abandon(f.Ctx, wire.TriageAbandonRequest{Project: "proj", Revision: rec.Revision})
	if err != nil || !res.Refused || res.Reason != wire.ReasonTriagePhase {
		t.Fatalf("abandon at write = %+v, %v", res, err)
	}
	if live, _ := store.LoadLive(); live == nil {
		t.Fatal("record left live state at write")
	}
}
