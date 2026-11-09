package triage

import (
	"errors"
	"sync"
	"testing"
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
	if rec.Step != "plan" || rec.Rows[1].Write != WriteGone {
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
	rec, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "map", Destination: "m"}}, nil)
	must(t, rec, err)
	if rec.Step != "plan" || rec.Rows[0].Plan == nil || rec.Rows[0].Plan.Destination != "m" {
		t.Fatalf("partial plan: %+v", rec)
	}
	// B goes gone during Plan; a plan row naming it is left out
	rec, err = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowB, Action: "ready"}}, []string{rowB})
	must(t, rec, err)
	if rec.Rows[1].Plan != nil || rec.Rows[1].Write != WriteGone || rec.Step != "approve" {
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
	rec, err := s.SetPerson(rec.ID, rec.Revision, rowA, PersonFields{Action: "map", Destination: "d", Answers: map[string]string{"q": "a"}})
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

func TestWriteState(t *testing.T) {
	s, rec := atApprove(t)
	if _, err := s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, "", ""); err == nil {
		t.Fatal("write state accepted before write")
	}
	rec, _ = s.Approve(rec.ID, rec.Revision)
	rec, err := s.WriteRow(rec.ID, rec.Revision, rowA, WriteFailed, "", "server-not-running")
	must(t, rec, err)
	if rec.Rows[0].Refusal != "server-not-running" {
		t.Fatalf("row = %+v", rec.Rows[0])
	}
	rec, err = s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, "proj:map/01", "")
	must(t, rec, err)
	if rec.Rows[0].Refusal != "" || rec.Rows[0].NewAddress != "proj:map/01" {
		t.Fatalf("a successful rerun should clear the refusal: %+v", rec.Rows[0])
	}
	if _, err := s.WriteRow(rec.ID, rec.Revision, rowA, "bogus", "", ""); err == nil {
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
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteFailed, "", "boom")
	rec, err := s.Retry(rec.ID, rec.Revision, rowA)
	must(t, rec, err)
	if rec.Rows[0].Write != WritePending {
		t.Fatalf("row = %+v", rec.Rows[0])
	}

	// failed after a move: Keep for later refused
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteFailed, "proj:map/01", "boom")
	if _, err := s.KeepForLater(rec.ID, rec.Revision, rowA); err == nil {
		t.Fatal("keep accepted after a move")
	}
	// failed in follow-ups: accepted
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteFailed, "", "boom")
	rec, err = s.KeepForLater(rec.ID, rec.Revision, rowB)
	must(t, rec, err)
	if !rec.Rows[1].Person.KeepForLater || rec.Rows[1].Write != WriteFailed {
		t.Fatalf("keep clears nothing: %+v", rec.Rows[1])
	}
}

func TestDoneRule(t *testing.T) {
	t.Run("gone during Write", func(t *testing.T) {
		s, rec := atWrite(t)
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, "", "")
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteGone, "", "")
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
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteFailed, "", "boom")
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
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, "", "")
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteDone, "", "")
		if rec.State != StateLive || rec.Step != "write" {
			t.Fatalf("crash before the last pass must leave it not done: %+v", rec)
		}
		if live, _ := s.LoadLive(); live == nil {
			t.Fatal("marker released early")
		}
	})
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
	good := Question{ID: "q1", Text: "Fix it where?", Kind: QuestionQuick, Options: []string{"parser", "caller"}, Default: "parser"}
	cases := map[string]Question{
		"no options":        {ID: "q1", Text: "Fix it where?", Kind: QuestionQuick, Default: "parser"},
		"one option":        {ID: "q1", Text: "Fix it where?", Kind: QuestionQuick, Options: []string{"parser"}, Default: "parser"},
		"five options":      {ID: "q1", Text: "t", Kind: QuestionQuick, Options: []string{"a", "b", "c", "d", "e"}, Default: "a"},
		"no default":        {ID: "q1", Text: "t", Kind: QuestionQuick, Options: []string{"a", "b"}},
		"default off list":  {ID: "q1", Text: "t", Kind: QuestionQuick, Options: []string{"a", "b"}, Default: "Use the recommended answer in the question text"},
		"duplicate options": {ID: "q1", Text: "t", Kind: QuestionQuick, Options: []string{"a", "a"}, Default: "a"},
	}
	for name, q := range cases {
		t.Run(name, func(t *testing.T) {
			s, rec := atPlan(t)
			if _, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready", Questions: []Question{q}}, {RowID: rowB, Action: "close"}}, nil); err == nil {
				t.Fatal("want the plan refused")
			}
		})
	}
	s, rec := atPlan(t)
	design := Question{ID: "d1", Text: "Which store?", Kind: QuestionDesign}
	must(t, rec, nil)
	if _, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "map", Questions: []Question{good, design}}, {RowID: rowB, Action: "close"}}, nil); err != nil {
		t.Fatalf("a quick question with options and a design question: %v", err)
	}
}

func TestPlan_EveryVerdictQuestionComesBackInThePlan(t *testing.T) {
	s, rec := project(t)
	rec, err := s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowA, Verdict: VerdictRelevant, Questions: []Question{{ID: "q1", Text: "raw?", Kind: QuestionQuick}}})
	rec = must(t, rec, err)
	rec, err = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowB, Verdict: VerdictRelevant})
	rec = must(t, rec, err)
	if _, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready"}, {RowID: rowB, Action: "close"}}, nil); err == nil {
		t.Fatal("want the plan refused: the verdict question has no plan question")
	}
	q := Question{ID: "q1", Text: "Fix it where?", Kind: QuestionQuick, Options: []string{"parser", "caller"}, Default: "parser"}
	rec, err = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready", Questions: []Question{q}}, {RowID: rowB, Action: "close"}}, nil)
	rec = must(t, rec, err)
	got := rec.Rows[0].AllQuestions()
	if len(got) != 1 || got[0].Text != "Fix it where?" || got[0].Default != "parser" {
		t.Fatalf("the plan question should replace the verdict's: %+v", got)
	}
}

func TestSendBack_KeepsTheAnswersAsEarlierRoundsAndReturnsToPlan(t *testing.T) {
	s, rec := atPlan(t)
	q := Question{ID: "q1", Text: "Fix it where?", Kind: QuestionQuick, Options: []string{"parser", "caller"}, Default: "parser"}
	rec, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready", Destination: "x", Questions: []Question{q}}, {RowID: rowB, Action: "close"}}, nil)
	rec = must(t, rec, err)
	if _, err := s.SendBack(rec.ID, rec.Revision); err == nil {
		t.Fatal("want send back refused while a question is open")
	}
	rec, err = s.SetPerson(rec.ID, rec.Revision, rowA, PersonFields{Action: "map", Destination: "m", Answers: map[string]string{"q1": "caller\nNote: see 12"}})
	rec = must(t, rec, err)
	rec, err = s.SendBack(rec.ID, rec.Revision)
	rec = must(t, rec, err)
	a := rec.Rows[0]
	if rec.Step != "plan" || rec.Round != 1 || a.Plan != nil || len(a.Questions) != 0 || len(a.Person.Answers) != 0 {
		t.Fatalf("step %s round %d row %+v", rec.Step, rec.Round, a)
	}
	if len(a.Earlier) != 1 || a.Earlier[0] != (Answered{Text: "Fix it where?", Answer: "caller\nNote: see 12"}) {
		t.Fatalf("earlier = %+v", a.Earlier)
	}
	if a.Person.Action != "map" || a.Person.Destination != "m" {
		t.Fatalf("the person's action and destination are kept: %+v", a.Person)
	}
	if Next(rec) != BranchPlan {
		t.Fatalf("next = %s, want plan", Next(rec))
	}
	// The agent plans again; the new round starts with no answers and approve is back.
	q2 := Question{ID: "q1", Text: "Which caller?", Kind: QuestionQuick, Options: []string{"a", "b"}, Default: "a"}
	rec, err = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready", Questions: []Question{q2}}, {RowID: rowB, Action: "close"}}, nil)
	rec = must(t, rec, err)
	if rec.Step != "approve" || rec.OpenQuestions() != 1 {
		t.Fatalf("step %s open %d", rec.Step, rec.OpenQuestions())
	}
	if _, err := s.SendBack(rec.ID, rec.Revision); err == nil {
		t.Fatal("want send back refused again while the new question is open")
	}
}

func TestSendBack_OnlyAtApprove(t *testing.T) {
	s, rec := atPlan(t)
	if _, err := s.SendBack(rec.ID, rec.Revision); err == nil {
		t.Fatal("want send back refused at plan")
	}
	s, rec = atWrite(t)
	if _, err := s.SendBack(rec.ID, rec.Revision); err == nil {
		t.Fatal("want send back refused at write")
	}
}
