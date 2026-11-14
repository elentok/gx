package triage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/triage"
	"github.com/elentok/gx/server/wire"
	triagerec "github.com/elentok/gx/triage"
)

const (
	addrA = "proj:follow-ups/01"
	addrB = "proj:follow-ups/02"
)

func startViewFixture(t *testing.T) (rigs.DesignsFixture, *triagerec.Store) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "follow-ups", "01", "first", servertest.TicketOpts{})
	servertest.WriteTicketWith(t, store, "proj", "follow-ups", "02", "second", servertest.TicketOpts{})
	f := rigs.StartDesignsIn(t, store)
	return f, triagerec.NewStore(filepath.Join(store, "proj"))
}

func show(t *testing.T, f rigs.DesignsFixture) wire.TriageView {
	t.Helper()
	v, err := f.H.Client.Triage.Show(f.Ctx, "proj")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestView_ExposesTheActionsThatTakeAnEpic(t *testing.T) {
	f, _ := startViewFixture(t)
	if got, want := show(t, f).EpicActions, []string{"ready", "area", "map"}; !slices.Equal(got, want) {
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
	q := triagerec.Question{ID: "q1", Text: "Which area?\nmore detail", Kind: triagerec.QuestionAsk, Options: []string{"x", "y"}, Default: "x"}
	rec = mustRec(s.SavePlan(rec.ID, rec.Revision, []triagerec.PlanRow{
		{RowID: addrA, Action: "ready", Fallback: "core", Questions: []triagerec.Question{q}},
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

	rec = mustRec(s.WriteRow(rec.ID, rec.Revision, addrB, triagerec.WriteDone, nil, ""))
	rec = mustRec(s.WriteRow(rec.ID, rec.Revision, addrA, triagerec.WriteDone, nil, ""))
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

func TestView_DesignQuestionsTravelAndAreNotAsked(t *testing.T) {
	f, s := startViewFixture(t)
	rec := mustRec(s.Create())
	rec = mustRec(s.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addrA, Verdict: "relevant"}))
	rec = mustRec(s.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addrB, Verdict: "relevant"}))
	design := triagerec.Question{ID: "d1", Text: "Which store?", Kind: triagerec.QuestionDesign}
	mustRec(s.SavePlan(rec.ID, rec.Revision, []triagerec.PlanRow{
		{RowID: addrA, Action: "map", Questions: []triagerec.Question{design}},
		{RowID: addrB, Action: "close"},
	}, nil))
	v := show(t, f)
	if v.OpenQuestions != 0 || len(v.Rows[0].Questions) != 0 || len(v.Rows[0].Travels) != 1 || v.Rows[0].Travels[0].ID != "d1" {
		t.Fatalf("view = %+v", v.Rows[0])
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
	rec = mustRec(s.WriteRow(rec.ID, rec.Revision, addrA, triagerec.WriteFailed, nil, "refused"))
	rec = mustRec(s.WriteRow(rec.ID, rec.Revision, addrB, triagerec.WriteFailed, []string{"proj:ready/01", "proj:ready/02"}, "set failed"))
	v := show(t, f)
	a, b := v.Rows[0], v.Rows[1]
	if !a.CanKeep || !a.CanRetry || a.Refusal != "refused" {
		t.Fatalf("before the move: %+v", a)
	}
	if b.CanKeep || !b.CanRetry || len(b.NewAddresses) != 2 || b.NewAddresses[1] != "proj:ready/02" {
		t.Fatalf("after the move: %+v", b)
	}
	if v.CanAbandon {
		t.Fatal("abandon offered at write")
	}
}

func TestOfferRules(t *testing.T) {
	row := func(w triagerec.WriteState, newAddr string, keep bool) triagerec.Row {
		var addrs []string
		if newAddr != "" {
			addrs = []string{newAddr}
		}
		return triagerec.Row{Write: w, NewAddresses: addrs, Person: triagerec.PersonFields{KeepForLater: keep}}
	}
	cases := []struct {
		name               string
		live               bool
		step               string
		legacyWrite        bool
		row                triagerec.Row
		keep, retry, abort bool
	}{
		{"verify", true, "verify", false, row("pending", "", false), false, false, true},
		{"route", true, "route", false, row("pending", "", false), false, false, true},
		{"questions", true, "questions", false, row("pending", "", false), false, false, true},
		{"write", true, "write", false, row("pending", "", false), false, false, false},
		{"version 1 write", true, "write", true, row("pending", "", false), false, false, true},
		{"write failed before move", true, "write", false, row("failed", "", false), true, true, false},
		{"write failed after move", true, "write", false, row("failed", "proj:ready/01", false), false, true, false},
		{"write failed already kept", true, "write", false, row("failed", "", true), false, true, false},
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
		if got := triage.CanAbandon(c.live, c.step, c.legacyWrite); got != c.abort {
			t.Errorf("%s: CanAbandon = %v", c.name, got)
		}
	}
}

func TestView_OpenRoundAndFinalApproval(t *testing.T) {
	f, s := startViewFixture(t)
	rec := mustRec(s.Create())
	rec = mustRec(s.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addrA, Verdict: "relevant"}))
	rec = mustRec(s.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addrB, Verdict: "relevant"}))
	if v := show(t, f); v.FinalApproval || v.OpenRound != nil {
		t.Fatalf("route: %+v", v)
	}
	q := triagerec.Question{ID: "q1", RowID: addrA, Text: "Which area?", Kind: triagerec.QuestionAsk, Options: []string{"x", "y"}, Default: "x"}
	rec = mustRec(s.SavePlan(rec.ID, rec.Revision, []triagerec.PlanRow{
		{RowID: addrA, Action: "ready", Fallback: "core", Questions: []triagerec.Question{q}},
		{RowID: addrB, Action: "close"},
	}, nil))
	if v := show(t, f); v.FinalApproval || v.OpenRound != nil || stepStates(v) != "ddctt" {
		t.Fatalf("an ask question waits for a round: %+v", v)
	}

	rec = mustRec(s.SaveRound(rec.ID, rec.Revision, []triagerec.Question{q}, false))
	v := show(t, f)
	if v.OpenRound == nil || *v.OpenRound != (wire.TriageOpenRound{Number: 1, Questions: 1, Answered: 0}) || v.FinalApproval {
		t.Fatalf("open round: %+v", v.OpenRound)
	}

	rec = mustRec(s.SaveAnswer(rec.ID, rec.Revision, "q1", "x"))
	if v = show(t, f); v.OpenRound != nil || !v.FinalApproval || !v.CanAbandon {
		t.Fatalf("final approval: %+v", v)
	}

	mustRec(s.Approve(rec.ID, rec.Revision))
	if v = show(t, f); v.FinalApproval || v.CanAbandon {
		t.Fatalf("write: %+v", v)
	}
}

func writeOldEpic(t *testing.T, projectDir, name string, statuses ...string) {
	t.Helper()
	dir := filepath.Join(projectDir, name, "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nkind: map\n---\n\n# " + name + "\n\n## Notes\n\nTriaged from follow-ups on 2026-10-01\n"
	if err := os.WriteFile(filepath.Join(projectDir, name, "ticket.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for i, st := range statuses {
		id := string(rune('1'+i)) + "0"
		ticket := "---\nid: \"" + id + "\"\nstatus: " + st + "\ntype: research\n---\n\n# T" + id + "\n"
		if err := os.WriteFile(filepath.Join(dir, id+"-t.md"), []byte(ticket), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestView_SummaryCountsMovesAndWaiting(t *testing.T) {
	// The old epics exist before the server starts: its index scans the store then.
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "follow-ups", "01", "first", servertest.TicketOpts{})
	servertest.WriteTicketWith(t, store, "proj", "follow-ups", "02", "second", servertest.TicketOpts{})
	writeOldEpic(t, filepath.Join(store, "proj"), "movable", "open")
	writeOldEpic(t, filepath.Join(store, "proj"), "stuck", "claimed", "done")
	f := rigs.StartDesignsIn(t, store)
	s := triagerec.NewStore(filepath.Join(store, "proj"))
	rec := mustRec(s.Create())
	rec = mustRec(s.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addrA, Verdict: "relevant"}))
	rec = mustRec(s.SaveVerdict(rec.ID, rec.Revision, triagerec.Verdict{Address: addrB, Verdict: "relevant"}))
	rec = mustRec(s.SaveAreas(rec.ID, rec.Revision, []triagerec.Area{{Name: "core", Source: triagerec.AreaAgent}}))
	rec = mustRec(s.SaveMoves(rec.ID, rec.Revision, []triagerec.MoveUnit{{OldEpic: "movable", Area: "core"}}))
	v := show(t, f)
	wantWait := "waiting: stuck holds claimed proj:stuck/10"
	if m := v.Summary.Moves; len(m) != 1 || m[0].OldEpic != "movable" || m[0].Area != "core" || m[0].State != "pending" {
		t.Fatalf("moves = %+v", m)
	}
	if w := v.Summary.Waiting; len(w) != 1 || w[0] != wantWait {
		t.Fatalf("waiting = %v, want %q", w, wantWait)
	}

	mustRec(s.SavePlan(rec.ID, rec.Revision, []triagerec.PlanRow{
		{RowID: addrA, Action: triagerec.ActionArea, Destination: "core"},
		{RowID: addrB, Action: "close"},
	}, nil))
	v = show(t, f)
	if v.Summary.Actions["area"] != 1 || v.Summary.Actions["close"] != 1 || v.Summary.Areas["core"] != 1 {
		t.Fatalf("summary = %+v", v.Summary)
	}
}

func TestView_VersionOneRecordAtWriteWaitsForYou(t *testing.T) {
	f, s := startViewFixture(t)
	folder := filepath.Join(s.ProjectDir(), ".triage")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"version":1,"id":"v1","startedAt":"2026-01-01T00:00:00Z","project":"proj","state":"live","step":"write","revision":3,` +
		`"rows":[{"address":"` + addrA + `","title":"first","write":"pending"}]}`
	if err := os.WriteFile(filepath.Join(folder, "v1.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "live"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := show(t, f)
	if !v.CanAbandon || !v.LegacyWrite || v.StepNote != "Waiting for you" {
		t.Fatalf("version 1 at write: %+v", v)
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
