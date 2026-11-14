package triageplay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/triage"
)

// stubGx is a gx binary that logs every call and answers the reads the player
// makes from canned output, so a turn runs without a server.
const stubGx = `#!/bin/sh
echo "$@" >> "$LOG"
case "$*" in
  *"triage show"*) cat "$SHOW" ;;
  *"triage locate"*) echo '{"position":"follow-ups","address":"p:follow-ups/01"}' ;;
  *"tickets move"*) echo '{"to":"p:triage-ready/01"}' ;;
  *"triage epics"*) echo 'p:triage-ready known' ;;
esac
`

type stub struct {
	player Player
	log    string
}

// newStub makes a player over the stub gx whose triage show prints rec at next.
func newStub(t *testing.T, knobs Config, next triage.Branch, rec triage.Record) *stub {
	t.Helper()
	dir := t.TempDir()
	gx := filepath.Join(dir, "gx")
	if err := os.WriteFile(gx, []byte(stubGx), 0o755); err != nil {
		t.Fatal(err)
	}
	show, err := json.Marshal(triageShow{Record: &rec, Next: next})
	if err != nil {
		t.Fatal(err)
	}
	showPath := filepath.Join(dir, "show.json")
	if err := os.WriteFile(showPath, show, 0o644); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "log")
	return &stub{
		player: Player{
			Gx:      gx,
			Env:     []string{"LOG=" + log, "SHOW=" + showPath},
			Config:  knobs,
			Project: "p",
			Dir:     dir,
		},
		log: log,
	}
}

func (s *stub) calls(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(s.log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func (s *stub) count(t *testing.T, sub string) int {
	t.Helper()
	n := 0
	for _, c := range s.calls(t) {
		if strings.Contains(c, sub) {
			n++
		}
	}
	return n
}

func rows(n int) []triage.Row {
	var out []triage.Row
	for i := 1; i <= n; i++ {
		out = append(out, triage.Row{Address: "p:follow-ups/0" + string(rune('0'+i)), Title: "row"})
	}
	return out
}

func TestTriagePlayer_VerifyEndsWithTheRunEndLine(t *testing.T) {
	t.Parallel()
	s := newStub(t, Config{}, triage.BranchVerify, triage.Record{Rows: rows(4)})
	msg, err := s.player.Play()
	if err != nil {
		t.Fatal(err)
	}
	if !grillround.RunEndLine.MatchString(msg) || !strings.HasPrefix(msg, "✅ verify:") {
		t.Errorf("message = %q, want a verify run-end line", msg)
	}
	if got := s.count(t, "triage verdict"); got != 4 {
		t.Errorf("verdicts saved = %d, want 4", got)
	}
}

func TestTriagePlayer_VerifyBatchStopsAfterNRows(t *testing.T) {
	t.Parallel()
	s := newStub(t, Config{VerifyBatch: 3}, triage.BranchVerify, triage.Record{Rows: rows(5)})
	if _, err := s.player.Play(); err != nil {
		t.Fatal(err)
	}
	if got := s.count(t, "triage verdict"); got != 3 {
		t.Errorf("verdicts saved = %d, want the batch of 3", got)
	}
}

func TestRoutePlan_CoversEveryActionWithThreeQuestions(t *testing.T) {
	t.Parallel()
	plan := RoutePlan(&triage.Record{Rows: rows(6)}, []string{"parser", "webui"})
	actions := map[string]bool{}
	ask, design := 0, 0
	for _, p := range plan {
		actions[p.Action] = true
		for _, q := range p.Questions {
			if q.Kind == triage.QuestionAsk {
				ask++
			} else {
				design++
			}
		}
	}
	if len(actions) != 4 || ask != 2 || design != 1 {
		t.Errorf("actions %v, %d ask and %d design questions, want 4, 2 and 1", actions, ask, design)
	}
	if got := plan[1].Destination; got != NewArea {
		t.Errorf("row 2 goes to %q, want the new area %q", got, NewArea)
	}
}

func TestRoutePlan_AtTheAreaCapFilesUnderTheClosestArea(t *testing.T) {
	t.Parallel()
	areas := make([]string, tickets.MaxAreas)
	for i := range areas {
		areas[i] = string(rune('a' + i))
	}
	plan := RoutePlan(&triage.Record{Rows: rows(2)}, areas)
	if got := plan[1].Destination; got != "a" {
		t.Errorf("row 2 goes to %q, want the closest area a", got)
	}
	if q := NewAreaQuestion(&triage.Record{Rows: rows(2)}, areas); len(q.Options) != 3 {
		t.Errorf("options = %v, want create, file under and keep", q.Options)
	}
}

func TestNewAreaQuestion_WithNoAreaOffersTwoOptions(t *testing.T) {
	t.Parallel()
	if q := NewAreaQuestion(&triage.Record{Rows: rows(2)}, nil); len(q.Options) != 2 || q.Default != q.Options[0] {
		t.Errorf("question = %+v, want two options and the first recommended", q)
	}
}

func TestTriagePlayer_RouteSavesTheAreaAndThePlanAndPrintsTheCard(t *testing.T) {
	t.Parallel()
	s := newStub(t, Config{}, triage.BranchRoute, triage.Record{Rows: rows(6)})
	msg, err := s.player.Play()
	if err != nil {
		t.Fatal(err)
	}
	if grillround.RunEndLine.MatchString(msg) || !strings.Contains(msg, "Route summary:") {
		t.Errorf("message = %q, want the summary card and no run-end line", msg)
	}
	if s.count(t, "agent step questions") != 1 {
		t.Errorf("calls = %q, want Route to chain into Questions", s.calls(t))
	}
	if s.count(t, "triage area --project p --name "+NewArea+" --source agent") != 1 || s.count(t, "triage plan") != 1 {
		t.Errorf("calls = %q, want one area save and one plan save", s.calls(t))
	}
}

func questionsRecord() triage.Record {
	ask := func(id string) triage.Question {
		return triage.Question{ID: id, RowID: "p:follow-ups/01", Text: "Q " + id, Kind: triage.QuestionAsk, Options: []string{"a", "b"}, Default: "a"}
	}
	return triage.Record{
		Step: tickets.StepQuestions, Revision: 7, Rows: rows(2),
		Rounds:  []triage.Round{{Questions: []triage.Question{ask("q1"), ask("q2")}, Answers: map[string]string{"q1": "a"}}},
		Decided: []triage.Decided{{RowID: "p:follow-ups/01", Question: "Rename?", Choice: "yes"}},
	}
}

func TestTriagePlayer_QuestionsReprintsTheOpenRoundWithoutSavingAnother(t *testing.T) {
	t.Parallel()
	s := newStub(t, Config{}, triage.BranchQuestions, questionsRecord())
	msg, err := s.player.Play()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "[q2]") || strings.Contains(msg, "[q1]") || !strings.Contains(msg, "Decided unless you object:") || grillround.RunEndLine.MatchString(msg) {
		t.Errorf("message = %q, want only the waiting question and the decided list, no run-end line", msg)
	}
	if s.count(t, "triage round") != 0 {
		t.Errorf("calls = %q, want no new round", s.calls(t))
	}
}

func TestTriagePlayer_QuestionsWithNothingLeftPrintsTheFinalCard(t *testing.T) {
	t.Parallel()
	rec := questionsRecord()
	rec.Rounds[0].Answers["q2"] = "b"
	s := newStub(t, Config{}, triage.BranchQuestions, rec)
	msg, err := s.player.Play()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(msg, "Write this plan?") || s.count(t, "triage round") != 0 {
		t.Errorf("message = %q, calls = %q, want the final card and no round", msg, s.calls(t))
	}
}

func TestTriagePlayer_TypedMessagesRunTheirVerbs(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ typed, call string }{
		{"yes", "triage approve --project p --revision 7"},
		{"Move row 2 to webui", "triage correct --project p --row p:follow-ups/02 --action area --destination webui"},
		{"Move row 2 to ready", "triage correct --project p --row p:follow-ups/02 --action ready --fallback " + NewArea},
		{"Answer q2: b", "triage answer --project p --question q2 --answer b"},
		{"Object to decision 1", "triage object --project p --row p:follow-ups/01 --question Rename? --option yes --option something else"},
	} {
		t.Run(c.typed, func(t *testing.T) {
			t.Parallel()
			s := newStub(t, Config{}, triage.BranchQuestions, questionsRecord())
			s.player.Typed = c.typed
			if _, err := s.player.Play(); err != nil {
				t.Fatal(err)
			}
			if s.count(t, c.call) != 1 {
				t.Errorf("calls = %q, want %q once", s.calls(t), c.call)
			}
		})
	}
}

func TestTriagePlayer_AnsweringTheNewAreaQuestionConfirmsTheArea(t *testing.T) {
	t.Parallel()
	s := newStub(t, Config{}, triage.BranchQuestions, questionsRecord())
	s.player.Typed = "Answer " + NewAreaQuestionID + ": Create " + NewArea
	if _, err := s.player.Play(); err != nil {
		t.Fatal(err)
	}
	if s.count(t, "triage area --project p --name "+NewArea+" --source agent --confirm") != 1 {
		t.Errorf("calls = %q, want the area confirmed", s.calls(t))
	}
}

func TestTriagePlayer_AnUnknownTypedMessageGetsAHelpLine(t *testing.T) {
	t.Parallel()
	s := newStub(t, Config{}, triage.BranchQuestions, questionsRecord())
	s.player.Typed = "hello"
	msg, err := s.player.Play()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, PhraseYes) || grillround.RunEndLine.MatchString(msg) {
		t.Errorf("message = %q, want the phrases and no run-end line", msg)
	}
}

func TestTriagePlayer_FailOnceFailsTheFirstWriteAndNotTheRerun(t *testing.T) {
	t.Parallel()
	row := triage.Row{
		Address: "p:follow-ups/01", Verdict: "relevant",
		Plan: &triage.PlanRow{RowID: "p:follow-ups/01", Action: "ready", Destination: triageReadyEpic},
	}
	knobs := Config{FailBeforeMove: []string{row.Address}}

	first := newStub(t, knobs, triage.BranchWrite, triage.Record{Step: tickets.StepWrite, Rows: []triage.Row{row}})
	if _, err := first.player.Play(); err != nil {
		t.Fatal(err)
	}
	if first.count(t, "tickets move") != 0 || first.count(t, "--state failed --refusal "+RefusalBeforeMove) != 1 {
		t.Errorf("first write calls = %q, want a failed row and no move", first.calls(t))
	}

	row.Write, row.Refusal = triage.WritePending, RefusalBeforeMove
	rerun := newStub(t, knobs, triage.BranchWrite, triage.Record{Step: tickets.StepWrite, Rows: []triage.Row{row}})
	if _, err := rerun.player.Play(); err != nil {
		t.Fatal(err)
	}
	if rerun.count(t, "tickets move") != 1 || rerun.count(t, "--state done --new-address p:triage-ready/01") != 1 {
		t.Errorf("rerun calls = %q, want the move and a done row", rerun.calls(t))
	}
}

func TestTriagePlayer_FailAfterMoveKeepsTheNewAddress(t *testing.T) {
	t.Parallel()
	row := triage.Row{
		Address: "p:follow-ups/01", Verdict: "relevant",
		Plan: &triage.PlanRow{RowID: "p:follow-ups/01", Action: "ready", Destination: triageReadyEpic},
	}
	s := newStub(t, Config{FailAfterMove: []string{row.Address}}, triage.BranchWrite, triage.Record{Step: tickets.StepWrite, Rows: []triage.Row{row}})
	if _, err := s.player.Play(); err != nil {
		t.Fatal(err)
	}
	if s.count(t, "tickets move") != 1 || s.count(t, "--state failed --refusal "+RefusalAfterMove+" --new-address p:triage-ready/01") != 1 {
		t.Errorf("calls = %q, want the move, then a failed row with the new address", s.calls(t))
	}
}
