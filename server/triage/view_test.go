package triage_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/triage"
	"github.com/elentok/gx/server/wire"
	triagerec "github.com/elentok/gx/triage"
)

const (
	addrA = "proj:follow-ups/01"
	addrB = "proj:follow-ups/02"
)

func startViewFixture(t *testing.T) (servertest.DesignsFixture, *triagerec.Store) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "follow-ups", "01", "first", servertest.TicketOpts{})
	servertest.WriteTicketWith(t, store, "proj", "follow-ups", "02", "second", servertest.TicketOpts{})
	f := servertest.StartDesignsIn(t, store)
	return f, triagerec.NewStore(filepath.Join(store, "proj"))
}

func show(t *testing.T, f servertest.DesignsFixture) wire.TriageView {
	t.Helper()
	v, err := f.H.Client.Triage.Show(f.Ctx, "proj")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestView_ExposesTheActionsThatTakeAnEpic(t *testing.T) {
	f, _ := startViewFixture(t)
	if got, want := show(t, f).EpicActions, []string{"ready", "map"}; !slices.Equal(got, want) {
		t.Fatalf("epic_actions = %v, want %v", got, want)
	}
}

// mustRec panics on error:a Go call cannot forward a two-value result next
// to a *testing.T, and a panic fails the test with a trace.
func mustRec(rec *triagerec.Record, err error) *triagerec.Record {
	if err != nil {
		panic(err)
	}
	return rec
}

func stepStates(v wire.TriageView) string {
	out := ""
	for _, s := range v.Steps {
		out += string(s.State)[:1]
	}
	return out
}

func TestView_NoRecordIsNone(t *testing.T) {
	f, _ := startViewFixture(t)
	v := show(t, f)
	if v.State != wire.TriageNone || v.ID != "" || len(v.History) != 0 || len(v.Rows) != 0 || v.CanAbandon {
		t.Fatalf("unexpected none view: %+v", v)
	}
	if v.Flow != "triage" || len(v.Steps) != 5 || stepStates(v) != "ttttt" {
		t.Fatalf("steps = %+v", v.Steps)
	}
	if n := len(v.Destinations); n != 1 || v.Destinations[0].Kind != "new-map-epic" {
		t.Fatalf("destinations = %+v", v.Destinations)
	}
}

func TestView_RecordAtEachStep(t *testing.T) {
	f, s := startViewFixture(t)
	rec := mustRec(s.Create())

	v := show(t, f)
	if v.State != wire.TriageLive || v.StepNote != "0 of 2 verdicts saved" || stepStates(v) != "ctttt" || v.RingPercent != 0 || !v.CanAbandon {
		t.Fatalf("verify: %+v", v)
	}
	if len(v.Rows) != 2 || v.Rows[0].Address != addrA || v.Rows[0].Title == "" {
		t.Fatalf("rows = %+v", v.Rows)
	}

	rec = mustRec(s.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addrA, Verdict: "relevant", Evidence: "still there", Files: []string{"a.go"}, SuggestedDestination: "polish"}))
	v = show(t, f)
	if v.StepNote != "1 of 2 verdicts saved" || v.RingPercent != 12 || v.Rows[0].Evidence != "still there" || v.Destinations[0].Value != "polish" {
		t.Fatalf("verify 1/2: %+v", v)
	}

	rec = mustRec(s.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addrB, Verdict: "fixed"}))
	q := triagerec.Question{ID: "q1", Text: "Which area?\nmore detail", Kind: triagerec.QuestionQuick, Options: []string{"x", "y"}, Default: "x"}
	rec = mustRec(s.SavePlan(rec.ID, rec.Revision, []triagerec.PlanRow{
		{RowID: addrA, Action: "ready", Questions: []triagerec.Question{q}},
		{RowID: addrB, Action: "close", Destination: "fixed already"},
	}, nil))
	v = show(t, f)
	if v.StepNote != "" || v.RingPercent != 50 || stepStates(v) != "ddctt" || v.OpenQuestions != 1 || v.ActionCounts["ready"] != 1 || v.ActionCounts["close"] != 1 {
		t.Fatalf("approve: %+v", v)
	}
	if got := v.Rows[0].Questions[0]; got.Title != "Which area?" || got.Body != "more detail" || got.Recommended != "x" {
		t.Fatalf("question = %+v", got)
	}

	rec = mustRec(s.SetPerson(rec.ID, rec.Revision, addrA, triagerec.PersonFields{Action: "keep", Answers: map[string]string{"q1": "x"}}))
	v = show(t, f)
	if v.OpenQuestions != 0 || v.ChangedRows != 1 || !v.Rows[0].PersonChanged || v.Rows[0].Action != "keep" || v.ActionCounts["keep"] != 1 || v.Rows[0].Answers["q1"] != "x" {
		t.Fatalf("person: %+v", v)
	}

	rec = mustRec(s.Approve(rec.ID, rec.Revision))
	v = show(t, f)
	if v.StepNote != "rows written 0 of 2" || stepStates(v) != "dddct" || v.CanAbandon || v.RingPercent != 75 {
		t.Fatalf("write: %+v", v)
	}

	rec = mustRec(s.WriteRow(rec.ID, rec.Revision, addrB, triagerec.WriteDone, "", ""))
	rec = mustRec(s.WriteRow(rec.ID, rec.Revision, addrA, triagerec.WriteDone, "", ""))
	rec = mustRec(s.SetLastPass(rec.ID, rec.Revision, true))
	v = show(t, f)
	if v.State != wire.TriageDone || stepStates(v) != "ddddd" || v.RingPercent != 100 || v.Result == nil || v.CanAbandon {
		t.Fatalf("done: %+v", v)
	}
	groups := map[string]int{}
	for _, g := range v.Result.Groups {
		groups[g.Action] = len(g.Items)
	}
	if groups["keep"] != 1 || groups["close"] != 1 || v.Result.Groups[0].Items[0].Reason != "fixed already" {
		t.Fatalf("result = %+v", v.Result)
	}
	if len(v.History) != 1 || v.History[0].ID != rec.ID || v.History[0].State != wire.TriageDone {
		t.Fatalf("history = %+v", v.History)
	}
}

func TestView_AbandonedIsShownAsLatest(t *testing.T) {
	f, s := startViewFixture(t)
	rec := mustRec(s.Create())
	mustRec(s.Abandon(rec.ID, rec.Revision))
	v := show(t, f)
	if v.State != wire.TriageAbandoned || v.ID != rec.ID || v.CanAbandon || v.Result != nil || len(v.History) != 1 {
		t.Fatalf("abandoned: %+v", v)
	}
}

func TestView_HistoryIsNewestFirstAndLiveWins(t *testing.T) {
	f, s := startViewFixture(t)
	// Age each record by rewriting its start time, so no test waits on the clock.
	aged := func(age time.Duration) *triagerec.Record {
		rec := mustRec(s.Create())
		rec = mustRec(s.Abandon(rec.ID, rec.Revision))
		rec.StartedAt = rec.StartedAt.Add(-age)
		if err := s.Save(rec); err != nil {
			t.Fatal(err)
		}
		return rec
	}
	first, second := aged(2*time.Hour), aged(time.Hour)
	third := mustRec(s.Create())

	v := show(t, f)
	if v.ID != third.ID || v.State != wire.TriageLive || len(v.History) != 2 || v.History[0].ID != second.ID || v.History[1].ID != first.ID {
		t.Fatalf("view = %+v", v)
	}
}

func TestView_FailedRowsOfferKeepAndRetry(t *testing.T) {
	f, s := startViewFixture(t)
	rec := mustRec(s.Create())
	rec = mustRec(s.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addrA}))
	rec = mustRec(s.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addrB}))
	rec = mustRec(s.SavePlan(rec.ID, rec.Revision, []triagerec.PlanRow{{RowID: addrA, Action: "ready"}, {RowID: addrB, Action: "ready"}}, nil))
	rec = mustRec(s.Approve(rec.ID, rec.Revision))
	rec = mustRec(s.WriteRow(rec.ID, rec.Revision, addrA, triagerec.WriteFailed, "", "refused"))
	rec = mustRec(s.WriteRow(rec.ID, rec.Revision, addrB, triagerec.WriteFailed, "proj:ready/01", "set failed"))
	v := show(t, f)
	a, b := v.Rows[0], v.Rows[1]
	if !a.CanKeep || !a.CanRetry || a.Refusal != "refused" {
		t.Fatalf("before the move: %+v", a)
	}
	if b.CanKeep || !b.CanRetry || b.NewAddress != "proj:ready/01" {
		t.Fatalf("after the move: %+v", b)
	}
	if v.CanAbandon {
		t.Fatal("abandon offered at write")
	}
}

func TestOfferRules(t *testing.T) {
	row := func(w triagerec.WriteState, newAddr string, keep bool) triagerec.Row {
		return triagerec.Row{Write: w, NewAddress: newAddr, Person: triagerec.PersonFields{KeepForLater: keep}}
	}
	cases := []struct {
		name               string
		live               bool
		step               string
		stopped            bool
		row                triagerec.Row
		keep, retry, abort bool
	}{
		{"verify", true, "verify", false, row("pending", "", false), false, false, true},
		{"verify stopped", true, "verify", true, row("pending", "", false), false, false, true},
		{"plan stopped", true, "plan", true, row("pending", "", false), false, false, true},
		{"approve", true, "approve", false, row("pending", "", false), false, false, true},
		{"approve stopped", true, "approve", true, row("pending", "", false), false, false, false},
		{"write running", true, "write", false, row("pending", "", false), false, false, false},
		{"write stopped", true, "write", true, row("pending", "", false), false, false, false},
		{"write failed before move", true, "write", false, row("failed", "", false), true, true, false},
		{"write failed after move", true, "write", false, row("failed", "proj:ready/01", false), false, true, false},
		{"write failed already kept", true, "write", false, row("failed", "", true), false, true, false},
		{"write stopped with failed row", true, "write", true, row("failed", "", false), true, true, false},
		{"write done row", true, "write", false, row("done", "", false), false, false, false},
		{"done", false, "done", false, row("failed", "", false), false, false, false},
	}
	for _, c := range cases {
		if got := triage.CanKeep(c.live, c.step, c.row); got != c.keep {
			t.Errorf("%s: CanKeep = %v", c.name, got)
		}
		if got := triage.CanRetry(c.live, c.step, c.row); got != c.retry {
			t.Errorf("%s: CanRetry = %v", c.name, got)
		}
		if got := triage.CanAbandon(c.live, c.step, c.stopped); got != c.abort {
			t.Errorf("%s: CanAbandon = %v", c.name, got)
		}
	}
}

func TestView_WebRead(t *testing.T) {
	f, s := startViewFixture(t)
	mustRec(s.Create())
	code, body := servertest.WebDo(t, f.H, "GET", f.Base+"/v1/triage?project=proj")
	if code != 200 {
		t.Fatalf("GET /v1/triage = %d: %s", code, body)
	}
	var v wire.TriageView
	if err := json.Unmarshal([]byte(body), &v); err != nil || v.State != wire.TriageLive || len(v.Rows) != 2 {
		t.Fatalf("body = %s (err %v)", body, err)
	}
	if code, _ := servertest.WebDo(t, f.H, "GET", f.Base+"/v1/triage?project=nope"); code != 404 {
		t.Fatalf("unknown project = %d, want 404", code)
	}
}
