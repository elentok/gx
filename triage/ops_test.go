package triage

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/elentok/gx/tickets/tickettest"
)

const (
	rowA = "proj:follow-ups/01"
	rowB = "proj:follow-ups/02"
)

// project builds a Triage with two rows. Row addresses are set explicitly so
// tests do not depend on the snapshot's address format.
func project(t *testing.T) (*Store, *Record) {
	t.Helper()
	s, _ := newProject(t)
	rec, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	rec.Rows[0].Address, rec.Rows[1].Address = rowA, rowB
	cur, err := s.Load(rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	cur.Rows = rec.Rows
	if err := s.Save(cur); err != nil {
		t.Fatal(err)
	}
	return s, cur
}

func must(t *testing.T, rec *Record, err error) *Record {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func atPlan(t *testing.T) (*Store, *Record) {
	s, rec := project(t)
	rec = must(t, rec, nil)
	for _, a := range []string{rowA, rowB} {
		var err error
		rec, err = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: a, Verdict: VerdictRelevant})
		must(t, rec, err)
	}
	return s, rec
}

func atApprove(t *testing.T) (*Store, *Record) {
	s, rec := atPlan(t)
	rec, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready"}, {RowID: rowB, Action: "close"}}, nil)
	return s, must(t, rec, err)
}

func atWrite(t *testing.T) (*Store, *Record) {
	s, rec := atApprove(t)
	rec, err := s.Approve(rec.ID, rec.Revision)
	return s, must(t, rec, err)
}

func TestVerdict(t *testing.T) {
	s, rec := project(t)
	rec, err := s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowA, Evidence: "e", Files: []string{"a.go"}, SuggestedDestination: "map-x"})
	must(t, rec, err)
	if rec.Rows[0].Verdict != VerdictRelevant || rec.Rows[0].Evidence != "e" || rec.Step != "verify" {
		t.Fatalf("undecided should count as relevant and keep verify: %+v", rec.Rows[0])
	}
	if p := rec.PendingVerdicts(); len(p) != 1 || p[0].Address != rowB {
		t.Fatalf("pending = %+v", p)
	}
	// a fresh session reads the stored record and sees only what is missing
	fresh, _ := s.LoadLive()
	if p := fresh.PendingVerdicts(); len(p) != 1 || p[0].Address != rowB {
		t.Fatalf("fresh pending = %+v", p)
	}
	if _, err := s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowB, Verdict: "bogus"}); err == nil {
		t.Fatal("unknown verdict accepted")
	}
	if _, err := s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: "nope", Verdict: VerdictFixed}); err == nil {
		t.Fatal("unknown row accepted")
	}
	rec, err = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowB, Gone: true})
	must(t, rec, err)
	if rec.Step != "route" || rec.Rows[1].Write != WriteGone {
		t.Fatalf("last row (gone) should move the step to plan: %+v", rec)
	}
	if _, err := s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowA, Verdict: VerdictFixed}); err == nil {
		t.Fatal("verdict accepted after verify")
	}
}

func TestSecondSaveOnOneRevisionRefused(t *testing.T) {
	s, rec := project(t)
	rev := rec.Revision
	if _, err := s.SaveVerdict(rec.ID, rev, Verdict{Address: rowA}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveVerdict(rec.ID, rev, Verdict{Address: rowB}); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("err = %v, want stale", err)
	}
}

func TestPlan(t *testing.T) {
	s, rec := atPlan(t)
	if _, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "bogus"}}, nil); err == nil {
		t.Fatal("unknown action accepted")
	}
	rec, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: ActionBacklog, Destination: "m"}}, nil)
	must(t, rec, err)
	if rec.Step != "route" || rec.Rows[0].Plan == nil || rec.Rows[0].Plan.Destination != "m" {
		t.Fatalf("partial plan: %+v", rec)
	}
	// B goes gone during Plan; a plan row naming it is left out
	rec, err = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowB, Action: "ready"}}, []string{rowB})
	must(t, rec, err)
	if rec.Rows[1].Plan != nil || rec.Rows[1].Write != WriteGone || rec.Step != "questions" {
		t.Fatalf("gone row should be left out and last save move to approve: %+v", rec)
	}
	if _, err := s.SavePlan(rec.ID, rec.Revision, nil, nil); err == nil {
		t.Fatal("plan save accepted after plan")
	}
}

func TestPersonFields(t *testing.T) {
	s, rec := atPlan(t)
	if _, err := s.SetPerson(rec.ID, rec.Revision, rowA, PersonFields{Action: "keep"}); err == nil {
		t.Fatal("person fields accepted before approve")
	}
	rec, _ = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready"}, {RowID: rowB, Action: "ready"}}, nil)
	if _, err := s.SetPerson(rec.ID, rec.Revision, rowA, PersonFields{Action: "bogus"}); err == nil {
		t.Fatal("unknown action accepted")
	}
	rec, err := s.SetPerson(rec.ID, rec.Revision, rowA, PersonFields{Action: ActionBacklog, Destination: "d", Answers: map[string]string{"q": "a"}})
	must(t, rec, err)
	if rec.Rows[0].Person.Destination != "d" || rec.Rows[0].Person.Answers["q"] != "a" {
		t.Fatalf("person = %+v", rec.Rows[0].Person)
	}
	rec, _ = s.Approve(rec.ID, rec.Revision)
	if _, err := s.SetPerson(rec.ID, rec.Revision, rowA, PersonFields{Action: "keep"}); err == nil {
		t.Fatal("person fields accepted after approve")
	}
}

func TestApprove(t *testing.T) {
	s, rec := atApprove(t)
	rev := rec.Revision

	// a person-field save in between makes the approve stale
	if _, err := s.SetPerson(rec.ID, rev, rowA, PersonFields{Action: "keep"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approve(rec.ID, rev); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("err = %v, want stale", err)
	}
	cur, _ := s.LoadLive()
	won, err := s.Approve(cur.ID, cur.Revision)
	if err != nil || won.Step != "write" {
		t.Fatalf("approve = %+v, %v", won, err)
	}
	if _, err := s.Approve(cur.ID, cur.Revision); !errors.Is(err, ErrAlreadyApproved) {
		t.Fatalf("err = %v, want already approved", err)
	}
}

func TestApprove_BeforeApproveStepRefused(t *testing.T) {
	s, rec := atPlan(t)
	if _, err := s.Approve(rec.ID, rec.Revision); err == nil || errors.Is(err, ErrAlreadyApproved) {
		t.Fatalf("err = %v", err)
	}
}

func TestApprove_RaceFirstWinsSecondAlreadyApproved(t *testing.T) {
	s, rec := atApprove(t)
	const n = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	won, already, stale := 0, 0, 0
	for range n {
		wg.Go(func() {
			_, err := s.Approve(rec.ID, rec.Revision)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				won++
			case errors.Is(err, ErrAlreadyApproved):
				already++
			case errors.Is(err, ErrStaleRevision):
				stale++
			default:
				t.Errorf("err = %v", err)
			}
		})
	}
	wg.Wait()
	if won != 1 || already != n-1 || stale != 0 {
		t.Fatalf("won=%d already=%d stale=%d", won, already, stale)
	}
}

func addTicket(t *testing.T, s *Store, epic, id string) {
	t.Helper()
	tickettest.WriteFile(t, filepath.Join(s.ProjectDir(), epic, "issues", id+"-t.md"),
		"---\nid: \""+id+"\"\nstatus: open\ntype: implement\n---\n\n# T\n")
}

func archiveFollowUp(t *testing.T, s *Store) {
	t.Helper()
	archived := filepath.Join(s.ProjectDir(), "follow-ups-archived", "issues")
	if err := os.MkdirAll(archived, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(s.ProjectDir(), "follow-ups", "issues", "01-a.md"), filepath.Join(archived, "01-a.md")); err != nil {
		t.Fatal(err)
	}
}

// saveDoneRow stores row A as done directly, bypassing WriteRow's gate, so
// the record-level check can be tested on its own.
func saveDoneRow(t *testing.T, s *Store, rec *Record, addrs ...string) *Record {
	t.Helper()
	rec.Rows[0].Write, rec.Rows[0].NewAddresses = WriteDone, addrs
	rec.Rows[1].Write = WriteGone
	if err := s.Save(rec); err != nil {
		t.Fatal(err)
	}
	cur, err := s.Load(rec.ID)
	must(t, cur, err)
	return cur
}

func TestWriteState(t *testing.T) {
	s, rec := atApprove(t)
	if _, err := s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, nil, ""); err == nil {
		t.Fatal("write state accepted before write")
	}
	rec, _ = s.Approve(rec.ID, rec.Revision)
	rec, err := s.WriteRow(rec.ID, rec.Revision, rowA, WriteFailed, nil, "server-not-running")
	must(t, rec, err)
	if rec.Rows[0].Refusal != "server-not-running" {
		t.Fatalf("row = %+v", rec.Rows[0])
	}
	if _, err := s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, []string{"proj:map/01"}, ""); err == nil {
		t.Fatal("done accepted though the new ticket is missing")
	}
	addTicket(t, s, "map", "01")
	if _, err := s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, []string{"proj:map/01"}, ""); err == nil {
		t.Fatal("done accepted though the follow-up is still open")
	}
	archiveFollowUp(t, s)
	rec, err = s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, []string{"proj:map/01"}, "")
	must(t, rec, err)
	if rec.Rows[0].Refusal != "" || rec.Rows[0].FirstAddress() != "proj:map/01" {
		t.Fatalf("a successful rerun should clear the refusal: %+v", rec.Rows[0])
	}
	if _, err := s.WriteRow(rec.ID, rec.Revision, rowA, "bogus", nil, ""); err == nil {
		t.Fatal("unknown state accepted")
	}
}

func TestRetryAndKeepForLater(t *testing.T) {
	s, rec := atApprove(t)
	if _, err := s.Retry(rec.ID, rec.Revision, rowA); err == nil {
		t.Fatal("retry accepted before write")
	}
	if _, err := s.KeepForLater(rec.ID, rec.Revision, rowA); err == nil {
		t.Fatal("keep accepted before write")
	}
	rec, _ = s.Approve(rec.ID, rec.Revision)
	if _, err := s.Retry(rec.ID, rec.Revision, rowA); err == nil {
		t.Fatal("retry of a pending row accepted")
	}
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteFailed, nil, "boom")
	rec, err := s.Retry(rec.ID, rec.Revision, rowA)
	must(t, rec, err)
	if rec.Rows[0].Write != WritePending {
		t.Fatalf("row = %+v", rec.Rows[0])
	}

	// failed after a move: Keep for later refused
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteFailed, []string{"proj:map/01"}, "boom")
	if _, err := s.KeepForLater(rec.ID, rec.Revision, rowA); err == nil {
		t.Fatal("keep accepted after a move")
	}
	// failed in follow-ups: accepted
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteFailed, nil, "boom")
	rec, err = s.KeepForLater(rec.ID, rec.Revision, rowB)
	must(t, rec, err)
	if !rec.Rows[1].Person.KeepForLater || rec.Rows[1].Write != WriteFailed {
		t.Fatalf("keep clears nothing: %+v", rec.Rows[1])
	}
}

func TestDoneRule(t *testing.T) {
	t.Run("gone during Write", func(t *testing.T) {
		s, rec := atWrite(t)
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, nil, "")
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteGone, nil, "")
		if rec.State != StateLive {
			t.Fatal("done before the last pass")
		}
		rec, err := s.SetLastPass(rec.ID, rec.Revision, true)
		must(t, rec, err)
		assertDone(t, s, rec)
	})
	t.Run("gone during Verify, kept row", func(t *testing.T) {
		s, rec := project(t)
		rec, _ = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowA, Gone: true})
		rec, _ = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowB})
		rec, _ = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowB, Action: "ready"}}, nil)
		rec, _ = s.Approve(rec.ID, rec.Revision)
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteFailed, nil, "boom")
		rec, _ = s.SetLastPass(rec.ID, rec.Revision, true)
		if rec.State != StateLive {
			t.Fatal("failed row should not be done")
		}
		rec, err := s.KeepForLater(rec.ID, rec.Revision, rowB)
		must(t, rec, err)
		assertDone(t, s, rec)
	})
	t.Run("flag unset", func(t *testing.T) {
		s, rec := atWrite(t)
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, nil, "")
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteDone, nil, "")
		if rec.State != StateLive || rec.Step != "write" {
			t.Fatalf("crash before the last pass must leave it not done: %+v", rec)
		}
		if live, _ := s.LoadLive(); live == nil {
			t.Fatal("marker released early")
		}
	})
}

func TestWriteRow_ListNeedsEveryTicketAndTheArchivedFollowUp(t *testing.T) {
	s, rec := atWrite(t)
	rec = saveDoneRow(t, s, rec, "proj:map-x/01", "proj:map-x/02")
	newTicket := func(id string) { addTicket(t, s, "map-x", id) }
	settle := func() *Record {
		cur, err := s.Load(rec.ID)
		must(t, cur, err)
		got, err := s.SetLastPass(cur.ID, cur.Revision, true)
		must(t, got, err)
		return got
	}

	newTicket("01")
	if got := settle(); got.State != StateLive {
		t.Fatal("done with only one of two tickets")
	}
	newTicket("02")
	if got := settle(); got.State != StateLive {
		t.Fatal("done while the follow-up is still in follow-ups")
	}
	archiveFollowUp(t, s)
	assertDone(t, s, settle())
}

func TestWriteRow_FollowUpMovedButNotArchivedIsNotDone(t *testing.T) {
	s, rec := atWrite(t)
	rec = saveDoneRow(t, s, rec, "proj:map-x/01")
	addTicket(t, s, "map-x", "01")
	elsewhere := filepath.Join(s.ProjectDir(), "elsewhere", "issues")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(s.ProjectDir(), "follow-ups", "issues", "01-a.md"), filepath.Join(elsewhere, "01-a.md")); err != nil {
		t.Fatal(err)
	}
	got, err := s.SetLastPass(rec.ID, rec.Revision, true)
	must(t, got, err)
	if got.State != StateLive {
		t.Fatalf("done though the follow-up only moved: %+v", got)
	}
}

func TestOldSingleNewAddressStillFinishes(t *testing.T) {
	s, rec := atWrite(t)
	rec.Rows[0].Write, rec.Rows[0].NewAddress = WriteDone, "proj:map/01"
	rec.Rows[1].Write = WriteGone
	if err := s.Save(rec); err != nil {
		t.Fatal(err)
	}
	live, _ := s.LoadLive()
	if got := live.Rows[0].NewAddresses; len(got) != 1 || got[0] != "proj:map/01" || live.Rows[0].NewAddress != "" {
		t.Fatalf("old address not normalised: %+v", live.Rows[0])
	}
	tickettest.WriteFile(t, filepath.Join(s.ProjectDir(), "map", "issues", "01-t.md"),
		"---\nid: \"01\"\nstatus: open\ntype: implement\n---\n\n# T\n")
	archived := filepath.Join(s.ProjectDir(), "follow-ups-archived", "issues")
	if err := os.MkdirAll(archived, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(s.ProjectDir(), "follow-ups", "issues", "01-a.md"), filepath.Join(archived, "01-a.md")); err != nil {
		t.Fatal(err)
	}
	rec, err := s.SetLastPass(live.ID, live.Revision, true)
	must(t, rec, err)
	assertDone(t, s, rec)
	if rec.Version != Version {
		t.Fatalf("version = %d", rec.Version)
	}
}

func assertDone(t *testing.T, s *Store, rec *Record) {
	t.Helper()
	if rec.State != StateDone || rec.Step != "done" {
		t.Fatalf("not done: %+v", rec)
	}
	if live, _ := s.LoadLive(); live != nil {
		t.Fatal("marker not released")
	}
}

func TestAbandon(t *testing.T) {
	cases := []struct {
		name string
		to   func(*testing.T) (*Store, *Record)
		ok   bool
	}{
		{"verify", project, true},
		{"plan", atPlan, true},
		{"approve", atApprove, true},
		{"write", atWrite, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, rec := c.to(t)
			got, err := s.Abandon(rec.ID, rec.Revision)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v, want ok=%v", err, c.ok)
			}
			if !c.ok {
				return
			}
			if got.State != StateAbandoned {
				t.Fatalf("state = %s", got.State)
			}
			if live, _ := s.LoadLive(); live != nil {
				t.Fatal("marker not released")
			}
			if h, _ := s.History(); len(h) != 1 {
				t.Fatalf("history = %d", len(h))
			}
		})
	}
}

func TestPlan_QuickQuestionsNeedOptionsAndADefault(t *testing.T) {
	good := Question{ID: "q1", Text: "Fix it where?", Kind: QuestionAsk, Options: []string{"parser", "caller"}, Default: "parser"}
	cases := map[string]Question{
		"no options":        {ID: "q1", Text: "Fix it where?", Kind: QuestionAsk, Default: "parser"},
		"one option":        {ID: "q1", Text: "Fix it where?", Kind: QuestionAsk, Options: []string{"parser"}, Default: "parser"},
		"five options":      {ID: "q1", Text: "t", Kind: QuestionAsk, Options: []string{"a", "b", "c", "d", "e"}, Default: "a"},
		"no default":        {ID: "q1", Text: "t", Kind: QuestionAsk, Options: []string{"a", "b"}},
		"default off list":  {ID: "q1", Text: "t", Kind: QuestionAsk, Options: []string{"a", "b"}, Default: "Use the recommended answer in the question text"},
		"duplicate options": {ID: "q1", Text: "t", Kind: QuestionAsk, Options: []string{"a", "a"}, Default: "a"},
	}
	for name, q := range cases {
		t.Run(name, func(t *testing.T) {
			s, rec := atPlan(t)
			if _, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready", Fallback: "core", Questions: []Question{q}}, {RowID: rowB, Action: "close"}}, nil); err == nil {
				t.Fatal("want the plan refused")
			}
		})
	}
	s, rec := atPlan(t)
	design := Question{ID: "d1", Text: "Which store?", Kind: QuestionDesign}
	must(t, rec, nil)
	if _, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: ActionBacklog, Destination: "m", Questions: []Question{good, design}}, {RowID: rowB, Action: "close"}}, nil); err != nil {
		t.Fatalf("a quick question with options and a design question: %v", err)
	}
}

func TestPlan_EveryVerdictQuestionComesBackInThePlan(t *testing.T) {
	s, rec := project(t)
	rec, err := s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowA, Verdict: VerdictRelevant, Questions: []Question{{ID: "q1", Text: "raw?", Kind: QuestionAsk, Options: []string{"a", "b"}}}})
	rec = must(t, rec, err)
	rec, err = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowB, Verdict: VerdictRelevant})
	rec = must(t, rec, err)
	if _, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready"}, {RowID: rowB, Action: "close"}}, nil); err == nil {
		t.Fatal("want the plan refused: the verdict question has no plan question")
	}
	q := Question{ID: "q1", Text: "Fix it where?", Kind: QuestionAsk, Options: []string{"parser", "caller"}, Default: "parser"}
	rec, err = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready", Fallback: "core", Questions: []Question{q}}, {RowID: rowB, Action: "close"}}, nil)
	rec = must(t, rec, err)
	got := rec.Rows[0].AllQuestions()
	if len(got) != 1 || got[0].Text != "Fix it where?" || got[0].Default != "parser" {
		t.Fatalf("the plan question should replace the verdict's: %+v", got)
	}
}

func TestPlan_ReadyRowWithAnAskQuestionNeedsAFallbackBacklog(t *testing.T) {
	s, rec := atPlan(t)
	ask := Question{ID: "q1", Text: "Fix it where?", Kind: QuestionAsk, Options: []string{"parser", "caller"}, Default: "parser"}
	if _, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready", Questions: []Question{ask}}, {RowID: rowB, Action: "close"}}, nil); err == nil {
		t.Fatal("want the plan refused without a fallback backlog")
	}
	rec, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready", Fallback: "core", Questions: []Question{ask}}, {RowID: rowB, Action: "close"}}, nil)
	rec = must(t, rec, err)
	if rec.Rows[0].Fallback != "core" || len(rec.Rows[0].Person.Answers) != 0 {
		t.Fatalf("row = %+v: the fallback is saved and no question is auto-answered", rec.Rows[0])
	}
}

func TestDesignQuestionsAreNotAskedAndDoNotBlockApprove(t *testing.T) {
	s, rec := atPlan(t)
	design := Question{ID: "d1", Text: "Which store?", Kind: QuestionDesign}
	rec, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: ActionBacklog, Destination: "m", Questions: []Question{design}}, {RowID: rowB, Action: "close"}}, nil)
	rec = must(t, rec, err)
	if got := rec.Rows[0].AskedQuestions(); len(got) != 0 {
		t.Fatalf("asked = %+v, want none", got)
	}
	if got := rec.Rows[0].AllQuestions(); len(got) != 1 {
		t.Fatalf("all = %+v, want the design question kept for Write", got)
	}
	if rec.OpenQuestions() != 0 {
		t.Fatalf("open = %d, want 0", rec.OpenQuestions())
	}
	if _, err := s.Approve(rec.ID, rec.Revision); err != nil {
		t.Fatalf("approve: %v", err)
	}
}

func TestSaveMoves_PendingAtRouteOnly(t *testing.T) {
	s, rec := atPlan(t)
	writeMoveEpics(t, s)
	if _, err := s.SaveMoves(rec.ID, rec.Revision, []MoveUnit{{OldEpic: "old", Backlog: "nowhere"}}); err == nil {
		t.Fatal("unknown backlog accepted")
	}
	rec, err := s.SaveMoves(rec.ID, rec.Revision, []MoveUnit{{OldEpic: "old", Backlog: "backlog", State: MoveDone}})
	must(t, rec, err)
	if len(rec.Moves) != 1 || rec.Moves[0].State != MovePending {
		t.Fatalf("moves = %+v", rec.Moves)
	}
	rec, _ = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready"}, {RowID: rowB, Action: "close"}}, nil)
	if _, err := s.SaveMoves(rec.ID, rec.Revision, nil); err == nil {
		t.Fatal("moves saved after route")
	}
}

func atWriteWithMove(t *testing.T) (*Store, *Record) {
	s, rec := atPlan(t)
	writeMoveEpics(t, s)
	rec, err := s.SaveMoves(rec.ID, rec.Revision, []MoveUnit{{OldEpic: "old", Backlog: "backlog"}})
	must(t, rec, err)
	rec, err = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready"}, {RowID: rowB, Action: "close"}}, nil)
	must(t, rec, err)
	rec, err = s.Approve(rec.ID, rec.Revision)
	return s, must(t, rec, err)
}

func TestMoveState_RetryKeepAndStep(t *testing.T) {
	s, rec := atWriteWithMove(t)
	if _, err := s.KeepForLater(rec.ID, rec.Revision, "old"); err == nil {
		t.Fatal("keep accepted on a pending unit")
	}
	if _, err := s.Retry(rec.ID, rec.Revision, "old"); err == nil {
		t.Fatal("retry accepted on a pending unit")
	}
	rec, err := s.SaveMoveState(rec.ID, rec.Revision, "old", MoveFailed, "needs-yes")
	must(t, rec, err)
	if rec.Moves[0].State != MoveFailed || rec.Moves[0].LastError != "needs-yes" {
		t.Fatalf("move = %+v", rec.Moves[0])
	}
	rec, err = s.Retry(rec.ID, rec.Revision, "old")
	must(t, rec, err)
	if rec.Moves[0].State != MovePending {
		t.Fatalf("move = %+v", rec.Moves[0])
	}
	rec, _ = s.SaveMoveState(rec.ID, rec.Revision, "old", MoveFailed, "boom")
	rec, err = s.KeepForLater(rec.ID, rec.Revision, "old")
	must(t, rec, err)
	if rec.Moves[0].State != MoveKept {
		t.Fatalf("move = %+v", rec.Moves[0])
	}
	if _, err := s.SaveMoveState(rec.ID, rec.Revision, "old", "bogus", ""); err == nil {
		t.Fatal("unknown state accepted")
	}
}

func writeKindEpic(t *testing.T, s *Store, name, kind, body string) {
	t.Helper()
	tickettest.WriteFile(t, filepath.Join(s.ProjectDir(), name, "ticket.md"), "---\nkind: "+kind+"\n---\n\n# "+name+"\n"+body)
}

// writeMoveEpics makes the old Triage epic "old" and the backlog "backlog".
func writeMoveEpics(t *testing.T, s *Store) {
	t.Helper()
	writeKindEpic(t, s, "old", "map", "\n## Notes\n\n"+OldEpicNote+" on 2026-10-01\n")
	writeKindEpic(t, s, "backlog", "backlog", "")
	addTicket(t, s, "old", "01")
	addTicket(t, s, "backlog", "01")
}

func TestSaveMoves_RefusesANonBacklogTargetAndANonCandidateOldEpic(t *testing.T) {
	s, rec := atPlan(t)
	writeMoveEpics(t, s)
	writeKindEpic(t, s, "other-map", "map", "")
	addTicket(t, s, "plain", "01")
	for name, u := range map[string]MoveUnit{
		"plain epic as backlog":   {OldEpic: "old", Backlog: "plain"},
		"map as backlog":          {OldEpic: "old", Backlog: "other-map"},
		"map without the note": {OldEpic: "other-map", Backlog: "backlog"},
		"plain epic as old":    {OldEpic: "plain", Backlog: "backlog"},
	} {
		if _, err := s.SaveMoves(rec.ID, rec.Revision, []MoveUnit{u}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSaveVerdict_RefusesABadQuestionKindOrAskOptions(t *testing.T) {
	s, rec := project(t)
	for name, q := range map[string]Question{
		"empty kind":   {ID: "q1", Text: "t"},
		"unknown kind": {ID: "q1", Text: "t", Kind: "quick"},
		"ask no opts":  {ID: "q1", Text: "t", Kind: QuestionAsk},
		"ask 5 opts":   {ID: "q1", Text: "t", Kind: QuestionAsk, Options: []string{"a", "b", "c", "d", "e"}},
	} {
		if _, err := s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowA, Questions: []Question{q}}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestMoveState_RefusedOutsideWrite(t *testing.T) {
	s, rec := atPlan(t)
	writeMoveEpics(t, s)
	rec, _ = s.SaveMoves(rec.ID, rec.Revision, []MoveUnit{{OldEpic: "old", Backlog: "backlog"}})
	if _, err := s.SaveMoveState(rec.ID, rec.Revision, "old", MoveDone, ""); err == nil {
		t.Fatal("move state accepted at route")
	}
	if _, err := s.Retry(rec.ID, rec.Revision, "old"); err == nil {
		t.Fatal("retry accepted at route")
	}
	if _, err := s.KeepForLater(rec.ID, rec.Revision, "old"); err == nil {
		t.Fatal("keep accepted at route")
	}
}

func TestDoneRule_EveryUnitDoneOrKept(t *testing.T) {
	s, rec := atWriteWithMove(t)
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, nil, "")
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteDone, nil, "")
	rec, _ = s.SetLastPass(rec.ID, rec.Revision, true)
	if rec.State != StateLive {
		t.Fatal("done with a pending unit")
	}
	rec, _ = s.SaveMoveState(rec.ID, rec.Revision, "old", MoveFailed, "boom")
	if rec.State != StateLive {
		t.Fatal("done with a failed unit")
	}
	rec, err := s.KeepForLater(rec.ID, rec.Revision, "old")
	must(t, rec, err)
	assertDone(t, s, rec)
}
