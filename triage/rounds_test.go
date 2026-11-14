package triage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func askQ(id, row string) Question {
	return Question{ID: id, RowID: row, Text: "Fix " + id + "?", Kind: QuestionAsk, Options: []string{"a", "b"}, Default: "a"}
}

func questions(row string, ids ...string) []Question {
	var out []Question
	for _, id := range ids {
		out = append(out, askQ(id, row))
	}
	return out
}

func answerAll(t *testing.T, s *Store, rec *Record, ids ...string) *Record {
	t.Helper()
	for _, id := range ids {
		var err error
		rec, err = s.SaveAnswer(rec.ID, rec.Revision, id, "a")
		must(t, rec, err)
	}
	return rec
}

func TestRound_CapsQuestionsAndOrdinaryRounds(t *testing.T) {
	s, rec := atApprove(t)
	if _, err := s.SaveRound(rec.ID, rec.Revision, questions(rowA, "1", "2", "3", "4", "5", "6", "7"), false); err == nil {
		t.Fatal("a round of 7 questions was accepted")
	}
	if _, err := s.SaveRound(rec.ID, rec.Revision, []Question{{ID: "d", RowID: rowA, Text: "t", Kind: QuestionDecide}}, false); err == nil {
		t.Fatal("a decide question reached a round")
	}
	for i := range MaxOrdinaryRounds {
		id := fmt.Sprintf("r%d", i)
		var err error
		rec, err = s.SaveRound(rec.ID, rec.Revision, questions(rowA, id), false)
		must(t, rec, err)
		rec = answerAll(t, s, rec, id)
	}
	if _, err := s.SaveRound(rec.ID, rec.Revision, questions(rowA, "late"), false); err == nil {
		t.Fatal("a fourth ordinary round was accepted")
	}
	rec, err := s.SaveRound(rec.ID, rec.Revision, questions(rowA, "o1", "o2", "o3", "o4", "o5", "o6"), true)
	must(t, rec, err)
	if !rec.Rounds[3].Objection {
		t.Fatalf("rounds = %+v", rec.Rounds)
	}
}

func TestRound_AnswersAndOpenRound(t *testing.T) {
	s, rec := atApprove(t)
	rec, err := s.SaveRound(rec.ID, rec.Revision, questions(rowA, "1", "2"), false)
	must(t, rec, err)
	for name, fn := range map[string]func() error{
		"unknown":  func() error { _, err := s.SaveAnswer(rec.ID, rec.Revision, "zz", "a"); return err },
		"empty":    func() error { _, err := s.SaveAnswer(rec.ID, rec.Revision, "1", "  "); return err },
		"a second": func() error { _, err := s.SaveRound(rec.ID, rec.Revision, questions(rowA, "3"), false); return err },
	} {
		if fn() == nil {
			t.Fatalf("%s: want a refusal", name)
		}
	}
	rec, err = s.SaveAnswer(rec.ID, rec.Revision, "1", "b")
	must(t, rec, err)
	_, open, ok := rec.OpenRound()
	if !ok || len(open) != 1 || open[0].ID != "2" || rec.Rounds[0].Answered {
		t.Fatalf("open = %+v ok=%v", open, ok)
	}
	if _, err := s.Approve(rec.ID, rec.Revision); err == nil {
		t.Fatal("approve accepted with an open round")
	}
	rec = answerAll(t, s, rec, "2")
	if _, _, ok := rec.OpenRound(); ok || !rec.Rounds[0].Answered {
		t.Fatalf("round should be answered: %+v", rec.Rounds[0])
	}
	// a question of an answered round is not the open one
	rec, err = s.SaveRound(rec.ID, rec.Revision, questions(rowA, "3"), false)
	must(t, rec, err)
	if _, err := s.SaveAnswer(rec.ID, rec.Revision, "1", "a"); err == nil {
		t.Fatal("answer saved into a round that is not the open one")
	}
}

func TestObjection_TurnsTheDecisionIntoAnAskQuestionInAnObjectionRound(t *testing.T) {
	s, rec := atApprove(t)
	rec, err := s.SaveDecision(rec.ID, rec.Revision, Decided{RowID: rowA, Question: "Keep flag?", Choice: "yes", Reason: "used"})
	must(t, rec, err)
	if rec.Decided[0].Objected {
		t.Fatal("a new decision is objected")
	}
	rec, err = s.Object(rec.ID, rec.Revision, rowA, "Keep flag?", []string{"yes", "no"})
	must(t, rec, err)
	if !rec.Decided[0].Objected || len(rec.Rounds) != 1 || !rec.Rounds[0].Objection {
		t.Fatalf("record = %+v", rec)
	}
	q := rec.Rounds[0].Questions[0]
	if q.Kind != QuestionAsk || q.Default != "yes" || q.RowID != rowA {
		t.Fatalf("question = %+v", q)
	}
	if _, err := s.Object(rec.ID, rec.Revision, rowA, "Keep flag?", []string{"yes", "no"}); err == nil {
		t.Fatal("a second objection to one decision was accepted")
	}
}

func TestObjection_RoundHoldsAtMostSix(t *testing.T) {
	s, rec := atApprove(t)
	for i := range MaxRoundQuestions + 1 {
		var err error
		rec, err = s.SaveDecision(rec.ID, rec.Revision, Decided{RowID: rowA, Question: fmt.Sprint("q", i), Choice: "yes"})
		must(t, rec, err)
	}
	for i := range MaxRoundQuestions {
		var err error
		rec, err = s.Object(rec.ID, rec.Revision, rowA, fmt.Sprint("q", i), []string{"yes", "no"})
		must(t, rec, err)
	}
	if _, err := s.Object(rec.ID, rec.Revision, rowA, fmt.Sprint("q", MaxRoundQuestions), []string{"yes", "no"}); err == nil {
		t.Fatal("a seventh objected question was accepted")
	}
}

func TestCorrect_ToAreaDropsUnansweredKeepsAnswers(t *testing.T) {
	s, rec := atApprove(t)
	rec, err := s.SaveRound(rec.ID, rec.Revision, questions(rowA, "1", "2"), false)
	must(t, rec, err)
	rec = answerAll(t, s, rec, "1")
	before := rec.Revision
	rec, err = s.Correct(rec.ID, rec.Revision, rowA, ActionArea, "core", "")
	must(t, rec, err)
	if rec.Revision == before {
		t.Fatal("the revision did not change")
	}
	rd := rec.Rounds[0]
	if len(rd.Questions) != 1 || rd.Questions[0].ID != "1" || rd.Answers["1"] != "a" || !rd.Answered {
		t.Fatalf("round = %+v", rd)
	}
	if _, _, ok := rec.OpenRound(); ok {
		t.Fatal("round still open")
	}
	if _, err := s.Correct(rec.ID, rec.Revision, rowA, ActionArea, "", ""); err == nil {
		t.Fatal("a move to an area without a name was accepted")
	}
}

func TestCorrect_ToReadyNeedsAFallbackAndQueuesTheAskQuestions(t *testing.T) {
	s, rec := project(t)
	for _, a := range []string{rowA, rowB} {
		var err error
		rec, err = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: a, Verdict: VerdictRelevant})
		must(t, rec, err)
	}
	q := askQ("q1", rowA)
	rec, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: ActionArea, Destination: "core", Questions: []Question{q}}, {RowID: rowB, Action: "close"}}, nil)
	must(t, rec, err)
	if got := rec.Unrounded(); len(got) != 0 {
		t.Fatalf("a row to an area has no candidates: %+v", got)
	}
	if _, err := s.Correct(rec.ID, rec.Revision, rowA, "ready", "", ""); err == nil {
		t.Fatal("a move to ready without a fallback was accepted")
	}
	before := rec.Revision
	rec, err = s.Correct(rec.ID, rec.Revision, rowA, "ready", "", "core")
	must(t, rec, err)
	if rec.Revision == before || rec.Rows[0].Fallback != "core" {
		t.Fatalf("row = %+v rev %d", rec.Rows[0], rec.Revision)
	}
	if got := rec.Unrounded(); len(got) != 1 || got[0].ID != "q1" {
		t.Fatalf("unrounded = %+v", got)
	}
}

func writeArea(t *testing.T, s *Store, name string) {
	t.Helper()
	dir := filepath.Join(s.ProjectDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ticket.md"), []byte("---\nkind: area\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAreas_ThirteenthIsRefused(t *testing.T) {
	s, rec := atPlan(t)
	for i := range 11 {
		writeArea(t, s, fmt.Sprintf("area-%d", i))
	}
	rec, err := s.SaveAreas(rec.ID, rec.Revision, []Area{{Name: "twelfth", Source: AreaAgent, Confirmed: true, Created: true}})
	must(t, rec, err)
	if _, err := s.SaveAreas(rec.ID, rec.Revision, []Area{{Name: "thirteenth", Source: AreaAgent, Confirmed: true, Created: true}}); err == nil {
		t.Fatal("a confirmed 13th area was accepted")
	}
	plan := []PlanRow{{RowID: rowA, Action: ActionArea, Destination: "thirteenth"}, {RowID: rowB, Action: "close"}}
	if _, err := s.SavePlan(rec.ID, rec.Revision, plan, nil); err == nil {
		t.Fatal("a plan creating a 13th area was accepted")
	}
	plan[0].Destination = "area-3"
	if _, err := s.SavePlan(rec.ID, rec.Revision, plan, nil); err != nil {
		t.Fatalf("a plan into an existing area: %v", err)
	}
}

func TestApprove_UnconfirmedAgentAreaRefusedTableAreaAccepted(t *testing.T) {
	s, rec := atPlan(t)
	rec, err := s.SaveAreas(rec.ID, rec.Revision, []Area{{Name: "core", Source: AreaTable}, {Name: "new", Source: AreaAgent, Created: true}})
	must(t, rec, err)
	rec, err = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: ActionArea, Destination: "core"}, {RowID: rowB, Action: "close"}}, nil)
	must(t, rec, err)
	if _, err := s.Approve(rec.ID, rec.Revision); err == nil {
		t.Fatal("approve accepted an unconfirmed agent area")
	}
	rec, err = s.SaveAreas(rec.ID, rec.Revision, []Area{{Name: "new", Source: AreaAgent, Confirmed: true, Created: true}})
	must(t, rec, err)
	if _, err := s.Approve(rec.ID, rec.Revision-1); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("err = %v, want stale", err)
	}
	rec, err = s.Approve(rec.ID, rec.Revision)
	if must(t, rec, err); rec.Step != "write" {
		t.Fatalf("step = %s", rec.Step)
	}
	if _, err := s.Approve(rec.ID, rec.Revision); !errors.Is(err, ErrAlreadyApproved) {
		t.Fatalf("err = %v, want already approved", err)
	}
}
