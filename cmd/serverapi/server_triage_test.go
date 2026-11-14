package serverapi

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	triagerec "github.com/elentok/gx/triage"
)

func TestServerTriageUngroup_UndoesTheGroupOnTheRevisionRead(t *testing.T) {
	dir := t.TempDir()
	servertest.WriteTicketWith(t, dir, "proj", "follow-ups", "01", "first", servertest.TicketOpts{})
	servertest.WriteTicketWith(t, dir, "proj", "follow-ups", "02", "second", servertest.TicketOpts{})
	f := rigs.StartDesignsIn(t, dir)
	store := triagerec.NewStore(filepath.Join(dir, "proj"))
	must := func(rec *triagerec.Record, err error) *triagerec.Record {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return rec
	}
	rec := must(store.Create())
	a, b := rec.Rows[0].Address, rec.Rows[1].Address
	for _, addr := range []string{a, b} {
		rec = must(store.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addr}))
	}
	rec = must(store.SavePlan(rec.ID, rec.Revision, []triagerec.PlanRow{
		{RowID: a, Action: "ready", Estimate: "10K", OverlapWith: []string{b}},
		{RowID: b, Action: "ready", Estimate: "10K", OverlapWith: []string{a}},
	}, nil))

	out, err := runServer(t, f.H, "triage", "ungroup", "proj", a, "--revision", "999", "--json")
	if err == nil && !strings.Contains(out, `"refused":true`) {
		t.Fatalf("stale ungroup = %q, %v, want a refusal", out, err)
	}
	if got := must(store.LoadLive()); got.Rows[0].Group == "" {
		t.Fatal("a stale ungroup undid the group")
	}

	if out, err := runServer(t, f.H, "triage", "ungroup", "proj", a, "--revision", strconv.Itoa(rec.Revision)); err != nil {
		t.Fatalf("ungroup = %q, %v", out, err)
	}
	if got := must(store.LoadLive()); got.Rows[0].Group != "" || got.Rows[1].Group != "" {
		t.Fatalf("rows after ungroup = %+v", got.Rows)
	}
}
