package triage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets/tickettest"
)

func TestRecordEpics_BacklogMustBeConfirmedAndIsKnownAfterwards(t *testing.T) {
	s, rec := atWrite(t)
	rec.Backlogs = []Backlog{
		{Name: "tbl", Source: BacklogTable},
		{Name: "ag", Source: BacklogAgent},
		{Name: "ag-ok", Source: BacklogAgent, Confirmed: true},
	}
	if err := s.Save(rec); err != nil {
		t.Fatal(err)
	}
	rec, err := s.LoadLive()
	must(t, rec, err)

	if _, err := s.RecordEpics(rec.ID, rec.Revision, "", nil, []string{"ag"}); err == nil {
		t.Fatal("unconfirmed agent backlog accepted")
	}
	if _, err := s.RecordEpics(rec.ID, rec.Revision, "", nil, []string{"nope"}); err == nil {
		t.Fatal("unsaved backlog accepted")
	}
	if rec.ListsEpic("tbl") {
		t.Fatal("backlog known before it was recorded")
	}
	rec, err = s.RecordEpics(rec.ID, rec.Revision, "", nil, []string{"tbl", "ag-ok"})
	must(t, rec, err)
	if !rec.ListsEpic("tbl") || !rec.ListsEpic("ag-ok") || rec.ListsEpic("ag") {
		t.Fatalf("backlogs = %+v", rec.Backlogs)
	}
}

func TestWriteRow_RowMovedIntoAnBacklogCanBeDone(t *testing.T) {
	s, rec := atWrite(t)
	tickettest.WriteFile(t, filepath.Join(s.ProjectDir(), "backlog-x", "issues", "03-t.md"),
		"---\nid: \"03\"\nstatus: open\ntype: implement\noriginal_address: proj:follow-ups/01\n---\n\n# T\n")
	if err := os.Remove(filepath.Join(s.ProjectDir(), "follow-ups", "issues", "01-a.md")); err != nil {
		t.Fatal(err)
	}
	rec, err := s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, []string{"proj:backlog-x/03"}, "")
	must(t, rec, err)
	if rec.Rows[0].Write != WriteDone {
		t.Fatalf("row = %+v", rec.Rows[0])
	}
}

func TestWriteTarget(t *testing.T) {
	ask := func(id, row string) Question {
		return Question{ID: id, RowID: row, Text: "Q " + id, Kind: QuestionAsk, Options: []string{"a", "b"}, Default: "a"}
	}
	rec := &Record{ReadyEpic: "ready-e", Rows: []Row{
		{Address: "p:follow-ups/01", Plan: &PlanRow{Action: ActionBacklog, Destination: "backlog-x"}, Questions: []Question{ask("q1", "p:follow-ups/01")}},
		{Address: "p:follow-ups/02", Plan: &PlanRow{Action: "ready", Fallback: "backlog-fb"}, Questions: []Question{ask("q2", "p:follow-ups/02")}},
		{Address: "p:follow-ups/03", Plan: &PlanRow{Action: "ready"}},
		{Address: "p:follow-ups/04", Plan: &PlanRow{Action: "close"}},
	}}

	got, _ := rec.WriteTarget(&rec.Rows[1])
	if got.Epic != "backlog-fb" || got.Type != TypeGrilling || !strings.Contains(got.Question, "Q q2") {
		t.Fatalf("unreached ready row = %+v", got)
	}
	if got, _ := rec.WriteTarget(&rec.Rows[2]); got.Epic != "ready-e" || got.Type != TypeImplement {
		t.Fatalf("plain ready row = %+v", got)
	}
	if _, ok := rec.WriteTarget(&rec.Rows[3]); ok {
		t.Fatal("close row has a target")
	}
	// a backlog row with an unrounded ask is grilling and carries the question, whatever the round count
	if got, _ := rec.WriteTarget(&rec.Rows[0]); got.Type != TypeGrilling || got.Epic != "backlog-x" || !strings.Contains(got.Question, "Q q1") {
		t.Fatalf("unrounded backlog row = %+v", got)
	}
	for range MaxOrdinaryRounds {
		rec.Rounds = append(rec.Rounds, Round{Questions: []Question{ask("other", "p:follow-ups/03")}})
	}
	if got, _ := rec.WriteTarget(&rec.Rows[0]); got.Type != TypeGrilling || !strings.Contains(got.Question, "Q q1") {
		t.Fatalf("unrounded backlog row after the rounds = %+v", got)
	}
}

func TestWriteTarget_BacklogRowTypes(t *testing.T) {
	rec := &Record{Rows: []Row{
		{Address: "p:follow-ups/01", Plan: &PlanRow{Action: ActionBacklog, Destination: "backlog-x", Investigate: true}},
		{Address: "p:follow-ups/02", Plan: &PlanRow{Action: ActionBacklog, Destination: "backlog-x"}},
		{Address: "p:follow-ups/03", Plan: &PlanRow{Action: ActionBacklog, Destination: "backlog-x", Investigate: true},
			Questions: []Question{{ID: "d1", RowID: "p:follow-ups/03", Text: "Which shape?", Kind: QuestionDesign}}},
	}}
	if got, _ := rec.WriteTarget(&rec.Rows[0]); got.Type != TypeResearch || got.Epic != "backlog-x" {
		t.Fatalf("investigate row = %+v", got)
	}
	if got, _ := rec.WriteTarget(&rec.Rows[1]); got.Type != TypeImplement {
		t.Fatalf("plain backlog row = %+v", got)
	}
	if got, _ := rec.WriteTarget(&rec.Rows[2]); got.Type != TypeGrilling || !strings.Contains(got.Question, "Which shape?") {
		t.Fatalf("design row = %+v", got)
	}
}

func TestDecisionLines_RoundsAndAgentDecisions(t *testing.T) {
	row := "p:follow-ups/01"
	rec := &Record{
		Rows: []Row{{Address: row}},
		Rounds: []Round{{
			Questions: []Question{{ID: "q1", RowID: row, Text: "Which?"}, {ID: "q2", RowID: row, Text: "Unanswered?"}},
			Answers:   map[string]string{"q1": "beta"},
		}},
		Decided: []Decided{{RowID: row, Question: "Where?", Choice: "here"}, {RowID: "other", Question: "No", Choice: "x"}},
	}
	got := rec.DecisionLines(&rec.Rows[0])
	want := []string{"- Which?: beta", "- Where?: here (decided by agent)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}
