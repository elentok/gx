package triage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets/tickettest"
)

func TestRecordEpics_AreaMustBeConfirmedAndIsKnownAfterwards(t *testing.T) {
	s, rec := atWrite(t)
	rec.Areas = []Area{
		{Name: "tbl", Source: AreaTable},
		{Name: "ag", Source: AreaAgent},
		{Name: "ag-ok", Source: AreaAgent, Confirmed: true},
	}
	if err := s.Save(rec); err != nil {
		t.Fatal(err)
	}
	rec, err := s.LoadLive()
	must(t, rec, err)

	if _, err := s.RecordEpics(rec.ID, rec.Revision, "", nil, []string{"ag"}); err == nil {
		t.Fatal("unconfirmed agent area accepted")
	}
	if _, err := s.RecordEpics(rec.ID, rec.Revision, "", nil, []string{"nope"}); err == nil {
		t.Fatal("unsaved area accepted")
	}
	if rec.ListsEpic("tbl") {
		t.Fatal("area known before it was recorded")
	}
	rec, err = s.RecordEpics(rec.ID, rec.Revision, "", nil, []string{"tbl", "ag-ok"})
	must(t, rec, err)
	if !rec.ListsEpic("tbl") || !rec.ListsEpic("ag-ok") || rec.ListsEpic("ag") {
		t.Fatalf("areas = %+v", rec.Areas)
	}
}

func TestWriteRow_RowMovedIntoAnAreaCanBeDone(t *testing.T) {
	s, rec := atWrite(t)
	tickettest.WriteFile(t, filepath.Join(s.ProjectDir(), "area-x", "issues", "03-t.md"),
		"---\nid: \"03\"\nstatus: open\ntype: implement\noriginal_address: proj:follow-ups/01\n---\n\n# T\n")
	if err := os.Remove(filepath.Join(s.ProjectDir(), "follow-ups", "issues", "01-a.md")); err != nil {
		t.Fatal(err)
	}
	rec, err := s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, []string{"proj:area-x/03"}, "")
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
		{Address: "p:follow-ups/01", Plan: &PlanRow{Action: ActionArea, Destination: "area-x"}, Questions: []Question{ask("q1", "p:follow-ups/01")}},
		{Address: "p:follow-ups/02", Plan: &PlanRow{Action: "ready", Fallback: "area-fb"}, Questions: []Question{ask("q2", "p:follow-ups/02")}},
		{Address: "p:follow-ups/03", Plan: &PlanRow{Action: "ready"}},
		{Address: "p:follow-ups/04", Plan: &PlanRow{Action: "close"}},
	}}

	got, _ := rec.WriteTarget(&rec.Rows[1])
	if got.Epic != "area-fb" || got.Type != TypeGrilling || !strings.Contains(got.Question, "Q q2") {
		t.Fatalf("unreached ready row = %+v", got)
	}
	if got, _ := rec.WriteTarget(&rec.Rows[2]); got.Epic != "ready-e" || got.Type != TypeImplement {
		t.Fatalf("plain ready row = %+v", got)
	}
	if _, ok := rec.WriteTarget(&rec.Rows[3]); ok {
		t.Fatal("close row has a target")
	}
	// an area row's questions are not asked; it only turns grilling after 3 rounds
	if got, _ := rec.WriteTarget(&rec.Rows[0]); got.Type != TypeImplement || got.Epic != "area-x" {
		t.Fatalf("area row before the rounds ran out = %+v", got)
	}
	for range MaxOrdinaryRounds {
		rec.Rounds = append(rec.Rounds, Round{Questions: []Question{ask("other", "p:follow-ups/03")}})
	}
	if got, _ := rec.WriteTarget(&rec.Rows[0]); got.Type != TypeGrilling || got.Epic != "area-x" || !strings.Contains(got.Question, "Q q1") {
		t.Fatalf("unreached area row = %+v", got)
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
