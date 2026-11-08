package triage_test

import (
	"sync"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	triagerec "github.com/elentok/gx/triage"
)

func approveReq(rec *triagerec.Record) wire.TriageApproveRequest {
	return wire.TriageApproveRequest{Project: "proj", Revision: rec.Revision}
}

func approve(t *testing.T, f servertest.DesignsFixture, req wire.TriageApproveRequest) wire.TriageEditResult {
	t.Helper()
	res, err := f.H.Client.Triage.Approve(f.Ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// atWrite approves the fixture directly through the record and fails row a.
func atWrite(t *testing.T, store *triagerec.Store) (*triagerec.Record, string, string) {
	t.Helper()
	rec, a, b := toApprove(t, store)
	rec = mustRec(store.Approve(rec.ID, rec.Revision))
	for _, addr := range []string{a, b} {
		rec = mustRec(store.WriteRow(rec.ID, rec.Revision, addr, triagerec.WriteFailed, "", "boom"))
	}
	return rec, a, b
}

func TestApprove_FirstWinsAndStartsOneWriteSession(t *testing.T) {
	f, store := startViewFixture(t)
	rec, _, _ := toApprove(t, store)
	req := approveReq(rec)
	if res := approve(t, f, req); res.Refused {
		t.Fatalf("first approve = %+v", res)
	}
	res := approve(t, f, req)
	if !res.Refused || res.Reason != wire.ReasonTriageAlreadyApproved {
		t.Fatalf("second approve = %+v, want already approved", res)
	}
	live := f.Live(t)
	if len(live) != 1 || live[0].Flow != "triage" {
		t.Fatalf("live sessions = %+v, want one triage session", live)
	}
	if got, _ := store.LoadLive(); got.Step != "write" {
		t.Fatalf("step = %s, want write", got.Step)
	}
}

func TestApprove_StaleRevisionIsRefused(t *testing.T) {
	f, store := startViewFixture(t)
	rec, a, _ := toApprove(t, store)
	if res := saveRow(t, f, wire.TriageRowRequest{Revision: rec.Revision, Address: a, Action: "map", Destination: "e"}); res.Refused {
		t.Fatalf("save = %+v", res)
	}
	res := approve(t, f, approveReq(rec))
	if !res.Refused || res.Reason != wire.ReasonStale {
		t.Fatalf("approve = %+v, want stale", res)
	}
	if len(f.Live(t)) != 0 {
		t.Fatal("a refused approve started a session")
	}
}

func TestApprove_OpenQuestionIsRefused(t *testing.T) {
	f, store := startViewFixture(t)
	rec := mustRec(store.Create())
	a, b := rec.Rows[0].Address, rec.Rows[1].Address
	rec = mustRec(store.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{
		Address: a, Questions: []triagerec.Question{{ID: "q1", Text: "which?", Kind: triagerec.QuestionQuick}},
	}))
	rec = mustRec(store.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: b}))
	rec = mustRec(store.SavePlan(rec.ID, rec.Revision, []triagerec.PlanRow{
		{RowID: a, Action: "ready"}, {RowID: b, Action: "ready"},
	}, nil))
	res := approve(t, f, approveReq(rec))
	if !res.Refused || res.Reason != wire.ReasonTriagePhase {
		t.Fatalf("approve = %+v, want refused while a question is open", res)
	}
	rec = mustRec(store.SetPerson(rec.ID, rec.Revision, a, triagerec.PersonFields{Answers: map[string]string{"q1": "x"}}))
	if res := approve(t, f, approveReq(rec)); res.Refused {
		t.Fatalf("approve after answering = %+v", res)
	}
}

func TestApprove_WebAndChatPathRaceToOneWinner(t *testing.T) {
	f, store := startViewFixture(t)
	rec, _, _ := toApprove(t, store)
	var wg sync.WaitGroup
	var webWon, chatWon bool
	wg.Add(2)
	go func() {
		defer wg.Done()
		webWon = !approve(t, f, approveReq(rec)).Refused
	}()
	go func() {
		defer wg.Done()
		_, err := store.Approve(rec.ID, rec.Revision)
		chatWon = err == nil
	}()
	wg.Wait()
	if webWon == chatWon {
		t.Fatalf("web won = %v, chat won = %v, want exactly one", webWon, chatWon)
	}
	wantSessions := 0
	if webWon {
		wantSessions = 1
	}
	if got := len(f.Live(t)); got != wantSessions {
		t.Fatalf("live sessions = %d, want %d", got, wantSessions)
	}
}

func rowAction(t *testing.T, f servertest.DesignsFixture, keep bool, rec *triagerec.Record, addr string) wire.TriageEditResult {
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
	rec = mustRec(store.WriteRow(rec.ID, rec.Revision, a, triagerec.WriteFailed, "other/01-x", "boom"))
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
	rec = mustRec(store.WriteRow(rec.ID, rec.Revision, a, triagerec.WriteDone, "", ""))
	rec = mustRec(store.SetLastPass(rec.ID, rec.Revision, true))
	if res := rowAction(t, f, true, rec, b); res.Refused {
		t.Fatalf("keep = %+v", res)
	}
	if live, _ := store.LoadLive(); live != nil {
		t.Fatalf("record still live: %+v", live)
	}
}
