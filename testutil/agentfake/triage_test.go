package agentfake

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
	player triagePlayer
	log    string
}

// newStub makes a player over the stub gx whose triage show prints rec at next.
func newStub(t *testing.T, knobs *TriageConfig, next triage.Branch, rec triage.Record) *stub {
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
		player: triagePlayer{
			cfg:     WorkerConfig{Gx: gx, Env: []string{"LOG=" + log, "SHOW=" + showPath}, Triage: knobs},
			project: "p",
			dir:     dir,
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
	s := newStub(t, nil, triage.BranchVerify, triage.Record{Rows: rows(4)})
	msg, err := s.player.play()
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
	s := newStub(t, &TriageConfig{VerifyBatch: 3}, triage.BranchVerify, triage.Record{Rows: rows(5)})
	if _, err := s.player.play(); err != nil {
		t.Fatal(err)
	}
	if got := s.count(t, "triage verdict"); got != 3 {
		t.Errorf("verdicts saved = %d, want the batch of 3", got)
	}
}

func TestTriagePlayer_PlanCoversEveryActionWithThreeQuestions(t *testing.T) {
	t.Parallel()
	plan := triagePlanRows(&triage.Record{Rows: rows(5)})
	actions := map[string]bool{}
	quick, design := 0, 0
	for _, p := range plan {
		actions[p.Action] = true
		for _, q := range p.Questions {
			if q.Kind == triage.QuestionQuick {
				quick++
			} else {
				design++
			}
		}
	}
	if len(actions) != 5 || quick != 2 || design != 1 {
		t.Errorf("actions %v, %d quick and %d design questions, want 5, 2 and 1", actions, quick, design)
	}
}

func TestTriagePlayer_FailOnceFailsTheFirstWriteAndNotTheRerun(t *testing.T) {
	t.Parallel()
	row := triage.Row{
		Address: "p:follow-ups/01", Verdict: "relevant",
		Plan: &triage.PlanRow{RowID: "p:follow-ups/01", Action: "ready", Destination: triageReadyEpic},
	}
	knobs := &TriageConfig{FailBeforeMove: []string{row.Address}}

	first := newStub(t, knobs, triage.BranchWrite, triage.Record{Step: tickets.StepWrite, Rows: []triage.Row{row}})
	if _, err := first.player.play(); err != nil {
		t.Fatal(err)
	}
	if first.count(t, "tickets move") != 0 || first.count(t, "--state failed --refusal "+TriageRefusalBeforeMove) != 1 {
		t.Errorf("first write calls = %q, want a failed row and no move", first.calls(t))
	}

	row.Write, row.Refusal = triage.WritePending, TriageRefusalBeforeMove
	rerun := newStub(t, knobs, triage.BranchWrite, triage.Record{Step: tickets.StepWrite, Rows: []triage.Row{row}})
	if _, err := rerun.player.play(); err != nil {
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
	s := newStub(t, &TriageConfig{FailAfterMove: []string{row.Address}}, triage.BranchWrite, triage.Record{Step: tickets.StepWrite, Rows: []triage.Row{row}})
	if _, err := s.player.play(); err != nil {
		t.Fatal(err)
	}
	if s.count(t, "tickets move") != 1 || s.count(t, "--state failed --refusal "+TriageRefusalAfterMove+" --new-address p:triage-ready/01") != 1 {
		t.Errorf("calls = %q, want the move, then a failed row with the new address", s.calls(t))
	}
}

func TestClassify_TriagePromptPlaysTriageAndAWorkerPromptIsUnchanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, WorkerConfigFile), []byte(`{"triage":{"verify_batch":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var tu tui
	tu.pid.Cwd = dir
	if kind, _, _ := tu.classify(TriagePromptPrefix + "--project p"); kind != promptTriage {
		t.Errorf("triage prompt kind = %v, want promptTriage", kind)
	}
	if kind, address, _ := tu.classify("/gx-implement p:epic/01"); kind != promptWorker || address != "p:epic/01" {
		t.Errorf("worker prompt = %v %q, want promptWorker p:epic/01", kind, address)
	}
}
