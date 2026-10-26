package server_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
)

// startWithLeftover starts a server on a fake runner whose ticket 01 label is
// held by a pane no run tracks, in state.
func startWithLeftover(t *testing.T, state agentrunner.State) (*servertest.Harness, *runnerfake.Runner, agentrunner.Session, string) {
	t.Helper()
	fake := runnerfake.NewRunner()
	leftover, err := fake.Start(agentrunner.StartOptions{Label: holderLabel, Cwd: "/elsewhere"})
	if err != nil {
		t.Fatal(err)
	}
	fake.SetState(holderLabel, state, "")
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.RunnerFor = func(_, _ string) agentrunner.Runner { return fake }
	})
	return h, fake, leftover, path
}

func TestRetryAndCancel_WorkingLeftoverRefuses(t *testing.T) {
	h, fake, _, path := startWithLeftover(t, agentrunner.StateWorking)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketRetry(ctx, "proj:epic-a/01", false)
	if err != nil || !res.Refused || res.Reason != server.ReasonAgentWorking {
		t.Errorf("retry = %+v, %v; want %s refusal", res, err, server.ReasonAgentWorking)
	}
	res, err = h.Client.TicketCancel(ctx, "proj:epic-a/01", false)
	if err != nil || !res.Refused || res.Reason != server.ReasonTicketLive {
		t.Errorf("cancel = %+v, %v; want %s refusal", res, err, server.ReasonTicketLive)
	}
	if _, ok, _ := fake.Find(holderLabel); !ok {
		t.Error("a refused verb closed the working leftover")
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "status: c") {
		t.Errorf("a refused verb changed the ticket:\n%s", data)
	}
}

func TestCancel_StopClosesALeftoverThroughTheRunner(t *testing.T) {
	h, fake, _, path := startWithLeftover(t, agentrunner.StateWorking)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", true); err != nil || res.Refused {
		t.Fatalf("cancel --stop = %+v, %v", res, err)
	}
	if _, ok, _ := fake.Find(holderLabel); ok {
		t.Error("the leftover is still live")
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "status: cancelled") {
		t.Errorf("ticket not cancelled:\n%s", data)
	}
}

func TestRetry_StopClosesAWorkingLeftover(t *testing.T) {
	h, fake, leftover, _ := startWithLeftover(t, agentrunner.StateWorking)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketRetry(ctx, "proj:epic-a/01", true); err != nil || res.Refused {
		t.Fatalf("retry --stop = %+v, %v", res, err)
	}
	if sess, ok, _ := fake.Find(holderLabel); ok && sess.ID == leftover.ID {
		t.Error("the working leftover is still live")
	}
}

// Retry on a repair pause reopens the ticket on the same branch: the head and
// its commits survive.
func TestRetry_RepairPauseKeepsTheBranchHead(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	const branch = "ralph-loop/epic-a-item-01"
	git("branch", branch)
	wt := filepath.Join(t.TempDir(), "wt")
	git("worktree", "add", wt, branch)
	testutil.WriteFile(t, wt, "work.txt", "work")
	testutil.CommitAll(t, wt, "agent work")
	git("worktree", "remove", "--force", wt)
	head := git("rev-parse", branch)

	h := servertest.StartWithStore(t, store, func(*server.Config) {})
	registerLaunch(h)
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketRetry(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("retry = %+v, %v", res, err)
	}
	if got := git("rev-parse", branch); got != head {
		t.Errorf("branch head = %s, want %s kept", got, head)
	}
}

func TestRelaunch_IdleLeftoverIsLeftToTheLaunch(t *testing.T) {
	h, _, _, _ := startWithLeftover(t, agentrunner.StateIdle)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("relaunch = %+v, %v", res, err)
	}
}

// A relaunch ends the ticket's open question with pause.closed{relaunch}; with no
// open question it writes nothing.
func TestRelaunch_ClosesTheOpenQuestion(t *testing.T) {
	h, _, _, path := startWithLeftover(t, agentrunner.StateIdle)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := tickets.AppendLog(path, tickets.LogEvent{Type: tickets.EventPauseQuestion, By: "gx", Summary: "q?"}); err != nil {
		t.Fatal(err)
	}

	if res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("relaunch = %+v, %v", res, err)
	}
	evs, err := tickets.ReadLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[1].Type != tickets.EventPauseClosed || evs[1].Question != 1 || evs[1].Reason != "relaunch" || evs[1].How != tickets.HowRelaunch || evs[1].Source != server.OriginCLI {
		t.Errorf("log = %+v, want the question then pause.closed{question 1, relaunch, cli}", evs)
	}
}

func TestRelaunch_RecoveryEndsTheRoundWithSourceGx(t *testing.T) {
	h, _, _, path := startWithLeftover(t, agentrunner.StateDone)
	if _, err := tickets.AppendLog(path, tickets.LogEvent{Type: tickets.EventPauseRepair, Summary: "stuck"}); err != nil {
		t.Fatal(err)
	}

	if _, err := h.Server.RecoveryDo(recovery.Call{Verb: recovery.VerbRelaunch, Address: "proj:epic-a/01"}); err != nil {
		t.Fatal(err)
	}
	rounds := endedRounds(t, path)
	if len(rounds) != 1 || rounds[0].End == nil || rounds[0].End.How != tickets.HowRelaunch || rounds[0].End.Source != tickets.SourceGx {
		t.Errorf("rounds = %+v; want the repair round ended by a gx relaunch", rounds)
	}
}

func TestRelaunch_WithoutQuestionWritesNoLog(t *testing.T) {
	h, _, _, path := startWithLeftover(t, agentrunner.StateIdle)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("relaunch = %+v, %v", res, err)
	}
	if evs, _ := tickets.ReadLog(path); len(evs) != 0 {
		t.Errorf("log = %+v, want empty", evs)
	}
}

func TestRelaunch_OnlyAHumanRelaunchIsLogged(t *testing.T) {
	h, _, _, _ := startWithLeftover(t, agentrunner.StateDone)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := h.Server.RecoveryDo(recovery.Call{Verb: recovery.VerbRelaunch, Address: "proj:epic-a/01"}); err != nil {
		t.Fatal(err)
	}
	if got := ofType(runLog(t, h), events.HumanRelaunch); len(got) != 0 {
		t.Fatalf("recovery relaunch logged %+v", got)
	}
	h.Server.DropRun("proj:epic-a/01")
	if res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01", true); err != nil || res.Refused {
		t.Fatalf("relaunch = %+v, %v", res, err)
	}
	if got := ofType(runLog(t, h), events.HumanRelaunch); len(got) != 1 || got[0].Ticket != "01" {
		t.Errorf("human-relaunch events = %+v, want one for 01", got)
	}
}

func TestGuardRail_AHumanRelaunchResetsTheBudget(t *testing.T) {
	applied := func(kind string) ralphloop.Event {
		return ralphloop.Event{Type: string(events.RecoveryApplied), Ticket: "01", Kind: kind, Reason: "relaunch"}
	}
	full := []ralphloop.Event{applied("a"), applied("b"), applied("c")}
	f := recovery.Failure{Address: "proj:epic-a/01", Type: events.Paused, Kind: "d"}
	if why := server.GuardRailStop(full, f); why == "" {
		t.Fatal("three recoveries did not stop recovery")
	}
	human := append(full, ralphloop.Event{Type: string(events.HumanRelaunch), Ticket: "01"})
	if why := server.GuardRailStop(human, f); why != "" {
		t.Errorf("after a human relaunch: %q, want recovery allowed", why)
	}
	if why := server.GuardRailStop(append(human, applied("d")), f); why == "" {
		t.Error("a recovery after the human relaunch was not counted")
	}
}
