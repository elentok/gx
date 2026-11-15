package triage_test

import (
	"testing"

	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	triagerec "github.com/elentok/gx/triage"
)

// atWrite approves the fixture directly through the record and fails row a.
func atWrite(t *testing.T, store *triagerec.Store) (*triagerec.Record, string, string) {
	t.Helper()
	rec, a, b := toApprove(t, store)
	rec = mustRec(store.Approve(rec.ID, rec.Revision))
	for _, addr := range []string{a, b} {
		rec = mustRec(store.WriteRow(rec.ID, rec.Revision, addr, triagerec.WriteFailed, nil, "boom"))
	}
	return rec, a, b
}

func rowAction(t *testing.T, f rigs.DesignsFixture, keep bool, rec *triagerec.Record, addr string) wire.TriageEditResult {
	t.Helper()
	req := wire.TriageRowActionRequest{Project: "proj", Revision: rec.Revision, Address: addr}
	call := f.H.Client.Triage.Retry
	if keep {
		call = f.H.Client.Triage.Keep
	}
	res, err := call(f.Ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestRetry_SetsPendingAndLaunchesOneWriteSession(t *testing.T) {
	f, store := startViewFixture(t)
	rec, a, b := atWrite(t, store)
	res := rowAction(t, f, false, rec, a)
	if res.Refused {
		t.Fatalf("retry = %+v", res)
	}
	if got := rowOf(show(t, f), a).WriteState; got != "pending" {
		t.Fatalf("write state = %q, want pending", got)
	}
	if len(f.Live(t)) != 1 {
		t.Fatalf("live sessions = %d, want 1", len(f.Live(t)))
	}
	cur, _ := store.LoadLive()
	if res := rowAction(t, f, false, cur, b); res.Refused {
		t.Fatalf("second retry = %+v", res)
	}
	if len(f.Live(t)) != 1 {
		t.Fatalf("live sessions = %d, want still 1", len(f.Live(t)))
	}
}

func TestKeep_RefusedAfterMoveAcceptedBefore(t *testing.T) {
	f, store := startViewFixture(t)
	rec, a, b := atWrite(t, store)
	rec = mustRec(store.WriteRow(rec.ID, rec.Revision, a, triagerec.WriteFailed, []string{"other/01-x"}, "boom"))
	res := rowAction(t, f, true, rec, a)
	if !res.Refused || res.Reason != wire.ReasonTriagePhase {
		t.Fatalf("keep after move = %+v, want refused", res)
	}
	if res := rowAction(t, f, true, rec, b); res.Refused {
		t.Fatalf("keep before move = %+v", res)
	}
}

func TestKeep_LastOpenRowWithLastPassCompleteMakesTheTriageDone(t *testing.T) {
	f, store := startViewFixture(t)
	rec, a, b := atWrite(t, store)
	rec = mustRec(store.WriteRow(rec.ID, rec.Revision, a, triagerec.WriteDone, nil, ""))
	rec = mustRec(store.SetLastPass(rec.ID, rec.Revision, true))
	if res := rowAction(t, f, true, rec, b); res.Refused {
		t.Fatalf("keep = %+v", res)
	}
	if live, _ := store.LoadLive(); live != nil {
		t.Fatalf("record still live: %+v", live)
	}
}
