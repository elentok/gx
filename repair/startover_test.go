package repair_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
)

// startOverFixture is a claimed ticket 01 of epic-a whose agent is live in
// runner, with no iteration branch or worktree to clear.
func startOverFixture(t *testing.T, state agentrunner.State) (repair.StartOverInput, ralphloop.Deps, *runnerfake.Runner, string) {
	t.Helper()
	epicPath := filepath.Join(t.TempDir(), "epic-a")
	if err := os.MkdirAll(filepath.Join(epicPath, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	ticket := "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n\n# 01 — first\n"
	if err := os.WriteFile(filepath.Join(epicPath, "issues", "01-first.md"), []byte(ticket), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := runnerfake.NewRunner()
	label, _, _ := ralphloop.IterationIdentity("epic-a", "01", "")
	if _, err := runner.Start(agentrunner.StartOptions{Label: label, Epic: "epic-a"}); err != nil {
		t.Fatal(err)
	}
	runner.SetState(label, state, "")
	d := ralphloop.Deps{
		Runner:         runner,
		WorktreeDir:    func(string) (string, error) { return t.TempDir(), nil },
		RevParse:       func(string, string) (string, error) { return "", git.ErrRefNotFound },
		WorktreeExists: func(string) (bool, error) { return false, nil },
	}
	in := repair.StartOverInput{EpicPath: epicPath, ID: "01", Reason: "stuck", Now: time.Now()}
	return in, d, runner, label
}

func TestStartOver_ClosesAnIdleOrDoneAgentAndStartsOver(t *testing.T) {
	for _, state := range []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone} {
		t.Run(string(state), func(t *testing.T) {
			in, d, runner, label := startOverFixture(t, state)
			res, err := repair.StartOver(in, d)
			if err != nil {
				t.Fatalf("StartOver: %v", err)
			}
			if res.Status != "open" {
				t.Errorf("status = %q, want open", res.Status)
			}
			if _, ok, _ := runner.Find(label); ok {
				t.Error("the agent's pane is still open")
			}
		})
	}
}

func TestStartOver_EndsTheOpenRoundWithTheRequestsSource(t *testing.T) {
	in, d, _, _ := startOverFixture(t, agentrunner.StateIdle)
	in.Source = tickets.SourceWeb
	path := filepath.Join(in.EpicPath, "issues", "01-first.md")
	if _, err := tickets.AppendLog(path, tickets.LogEvent{Type: tickets.EventPauseRepair, Summary: "stuck"}); err != nil {
		t.Fatal(err)
	}

	if _, err := repair.StartOver(in, d); err != nil {
		t.Fatalf("StartOver: %v", err)
	}
	evs, err := tickets.ReadLog(path)
	if err != nil {
		t.Fatal(err)
	}
	rounds := tickets.AllRounds(evs)
	if len(rounds) != 1 || rounds[0].End == nil || rounds[0].End.How != tickets.HowStartOver || rounds[0].End.Source != tickets.SourceWeb {
		t.Errorf("rounds = %+v; want one ended by a web start-over", rounds)
	}
}

func TestStartOver_RefusesAWorkingAgent(t *testing.T) {
	for _, state := range []agentrunner.State{agentrunner.StateWorking, agentrunner.StateBlocked} {
		t.Run(string(state), func(t *testing.T) {
			in, d, runner, label := startOverFixture(t, state)
			_, err := repair.StartOver(in, d)
			var r *repair.RefusalError
			if !errors.As(err, &r) || r.Reason != repair.ReasonLiveAgentOnTab {
				t.Fatalf("err = %v, want a %s refusal", err, repair.ReasonLiveAgentOnTab)
			}
			if _, ok, _ := runner.Find(label); !ok {
				t.Error("the working agent was stopped")
			}
		})
	}
}
