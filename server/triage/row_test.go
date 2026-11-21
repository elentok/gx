package triage_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
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
	if rec.Step != "questions" {
		t.Fatalf("step = %s, want questions", rec.Step)
	}
	return rec, a, b
}

func ungroup(t *testing.T, f rigs.DesignsFixture, revision int, address string) ops.TriageEditResult {
	t.Helper()
	res, err := f.H.Client.Triage.Ungroup(f.Ctx, ops.TriageUngroupRequest{Project: "proj", Revision: revision, Address: address})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func rowOf(v ops.TriageView, addr string) ops.TriageRow {
	for _, r := range v.Rows {
		if r.Address == addr {
			return r
		}
	}
	return ops.TriageRow{}
}

func TestUngroup_UndoesAGroupLikeTheCLIAndRefusesAStaleRevision(t *testing.T) {
	f, store := startViewFixture(t)
	rec := mustRec(store.Create())
	a, b := rec.Rows[0].Address, rec.Rows[1].Address
	for _, addr := range []string{a, b} {
		rec = mustRec(store.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addr}))
	}
	rec = mustRec(store.SavePlan(rec.ID, rec.Revision, []triagerec.PlanRow{
		{RowID: a, Action: "ready", Estimate: "10K", OverlapWith: []string{b}},
		{RowID: b, Action: "ready", Estimate: "10K", OverlapWith: []string{a}},
	}, nil))
	if rec.Rows[0].Group == "" {
		t.Fatal("rows were not grouped")
	}
	if res := ungroup(t, f, rec.Revision+5, a); !res.Refused || res.Reason != wire.ReasonStale {
		t.Fatalf("stale ungroup = %+v, want a stale-revision refusal", res)
	}
	if res := ungroup(t, f, rec.Revision, a); res.Refused {
		t.Fatalf("ungroup = %+v", res)
	}
	got := mustRec(store.LoadLive())
	if got.Rows[0].Group != "" || got.Rows[1].Group != "" || len(got.Rows[0].Plan.OverlapWith) != 1 {
		t.Fatalf("rows after ungroup = %+v", got.Rows)
	}
}

func TestView_GroupMembersShareTheMarkerAndUngroupedRowsShowTheirChain(t *testing.T) {
	f, store := startViewFixture(t)
	rec := mustRec(store.Create())
	a, b := rec.Rows[0].Address, rec.Rows[1].Address
	for _, addr := range []string{a, b} {
		rec = mustRec(store.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addr}))
	}
	rec = mustRec(store.SavePlan(rec.ID, rec.Revision, []triagerec.PlanRow{
		{RowID: a, Action: "ready", Estimate: "10K", OverlapWith: []string{b}},
		{RowID: b, Action: "ready", Estimate: "10K", OverlapWith: []string{a}},
	}, nil))
	v := show(t, f)
	if rowOf(v, a).Group == "" || rowOf(v, a).Group != rowOf(v, b).Group {
		t.Fatalf("groups = %q, %q, want one shared marker", rowOf(v, a).Group, rowOf(v, b).Group)
	}
	if res := ungroup(t, f, rec.Revision, a); res.Refused {
		t.Fatalf("ungroup = %+v", res)
	}
	v = show(t, f)
	if rowOf(v, a).Group != "" || len(rowOf(v, a).OverlapWith) != 1 || rowOf(v, a).OverlapWith[0] != b {
		t.Fatalf("row a after ungroup = %+v", rowOf(v, a))
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
	f := rigs.StartDesignsIn(t, root)
	store := triagerec.NewStore(filepath.Join(root, "proj"))
	sess := openTriage(t, f, ops.TriageOpenRequest{Project: "proj"}).Session
	servertest.WaitForHolder(t, filepath.Join(f.H.SessionsDir, sess.ID))
	rec, _ := store.LoadLive()
	for _, r := range rec.Rows {
		rec = mustRec(store.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: r.Address}))
	}
	before := ticketFiles(t, root)

	res, err := f.H.Client.Triage.Abandon(f.Ctx, ops.TriageAbandonRequest{Project: "proj", Revision: rec.Revision})
	if err != nil || res.Refused {
		t.Fatalf("abandon = %+v, %v", res, err)
	}
	if live, _ := store.LoadLive(); live != nil {
		t.Fatalf("record still live: %+v", live)
	}
	v := show(t, f)
	if v.State != ops.TriageAbandoned || len(v.History) != 1 || v.History[0].State != ops.TriageAbandoned {
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
	res, err := f.H.Client.Triage.Abandon(f.Ctx, ops.TriageAbandonRequest{Project: "proj", Revision: rec.Revision + 5})
	if err != nil || !res.Refused || res.Reason != wire.ReasonStale {
		t.Fatalf("stale abandon = %+v, %v", res, err)
	}
	rec = mustRec(store.Approve(rec.ID, rec.Revision))
	res, err = f.H.Client.Triage.Abandon(f.Ctx, ops.TriageAbandonRequest{Project: "proj", Revision: rec.Revision})
	if err != nil || !res.Refused || res.Reason != ops.ReasonTriagePhase {
		t.Fatalf("abandon at write = %+v, %v", res, err)
	}
	if live, _ := store.LoadLive(); live == nil {
		t.Fatal("record left live state at write")
	}
}
