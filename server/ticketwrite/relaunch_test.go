package ticketwrite_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestRetry_StopClosesAWorkingLeftover(t *testing.T) {
	t.Parallel()
	h, fake, leftover, _ := servertest.StartWithLeftover(t, agentrunner.StateWorking)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketRetry(ctx, "proj:epic-a/01", true); err != nil || res.Refused {
		t.Fatalf("retry --stop = %+v, %v", res, err)
	}
	if sess, ok, _ := fake.Find(servertest.HolderLabel); ok && sess.ID == leftover.ID {
		t.Error("the working leftover is still live")
	}
}

// Retry on a repair pause reopens the ticket on the same branch: the head and
// its commits survive.
func TestRetry_RepairPauseKeepsTheBranchHead(t *testing.T) {
	t.Parallel()
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
	servertest.RegisterTurn(h)
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
	t.Parallel()
	h, _, _, _ := servertest.StartWithLeftover(t, agentrunner.StateIdle)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("relaunch = %+v, %v", res, err)
	}
}

// A relaunch ends the ticket's open question with pause.closed{relaunch}; with no
// open question it writes nothing.
func TestRelaunch_ClosesTheOpenQuestion(t *testing.T) {
	t.Parallel()
	h, _, _, path := servertest.StartWithLeftover(t, agentrunner.StateIdle)
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
	if len(evs) != 2 || evs[1].Type != tickets.EventPauseClosed || evs[1].Question != 1 || evs[1].Reason != "relaunch" || evs[1].How != tickets.HowRelaunch || evs[1].Source != wire.OriginCLI {
		t.Errorf("log = %+v, want the question then pause.closed{question 1, relaunch, cli}", evs)
	}
}

func TestRelaunch_WithoutQuestionWritesNoLog(t *testing.T) {
	t.Parallel()
	h, _, _, path := servertest.StartWithLeftover(t, agentrunner.StateIdle)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("relaunch = %+v, %v", res, err)
	}
	if evs, _ := tickets.ReadLog(path); len(evs) != 0 {
		t.Errorf("log = %+v, want empty", evs)
	}
}
