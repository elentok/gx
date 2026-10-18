package server_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/herdrfake"
)

const iterLabel = "epic-a-iter-01"

// startOnRunner starts a server whose configured runner is a fake named name,
// with claimed ticket 01's agent live in it but no tracked run.
func startOnRunner(t *testing.T, name string, opts ...func(*server.Config)) (*servertest.Harness, renamedFake, agentrunner.Session) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	claimTicket(t, store, "epic-a", "01", "01-first.md")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	fake := newRenamedFake(name, nativerunner.VerdictLive)
	h := servertest.StartWithStore(t, store, append([]func(*server.Config){func(c *server.Config) { c.Runner = fake }}, opts...)...)
	sess, err := fake.Start(agentrunner.StartOptions{Label: iterLabel, Epic: "epic-a"})
	if err != nil {
		t.Fatal(err)
	}
	return h, fake, sess
}

func TestRecoveryRunner_HeadlessRunListsWithoutPaneOrTab(t *testing.T) {
	h, _, sess := startOnRunner(t, "headless")
	its, err := h.Client.Iterations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(its) != 1 {
		t.Fatalf("iterations = %+v, want one", its)
	}
	it := its[0]
	if it.Pane != "" || it.Tab != "" || it.Session != sess.SessionID || it.Branch == "" || it.Worktree == "" {
		t.Errorf("iteration = %+v, want session %s, worktree and branch, no pane or tab", it, sess.SessionID)
	}
}

func TestRecoveryRunner_LiveCheckAgreesAcrossShapes(t *testing.T) {
	for _, c := range []struct {
		runner string
		state  agentrunner.State
		live   bool
	}{
		{"headless", agentrunner.StateIdle, true},
		{"headless", agentrunner.StateBlocked, true},
		{"headless", agentrunner.StateDone, false},
		{"herdr", agentrunner.StateIdle, true},
		{"herdr", agentrunner.StateBlocked, true},
		// herdr's done is a finished turn resting in its pane.
		{"herdr", agentrunner.StateDone, true},
	} {
		t.Run(c.runner+"/"+string(c.state), func(t *testing.T) {
			h, fake, sess := startOnRunner(t, c.runner)
			h.Herdr.Register("agent", "get", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
				return map[string]any{"agent": map[string]any{"pane_id": sess.ID, "tab_id": "tab-1"}}, herdrfake.Identities{}, nil
			})
			fake.SetState(iterLabel, c.state, "")
			its, err := h.Client.Iterations(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if live := len(its) == 1; live != c.live {
				t.Fatalf("iterations = %+v, want live %v", its, c.live)
			}
			if !c.live {
				return
			}
			wantPane, wantTab := "", ""
			if c.runner == "herdr" {
				wantPane, wantTab = sess.ID, "tab-1"
			}
			if its[0].Pane != wantPane || its[0].Tab != wantTab {
				t.Errorf("pane/tab = %q/%q, want %q/%q", its[0].Pane, its[0].Tab, wantPane, wantTab)
			}
		})
	}
}

func TestRecoveryRunner_ClosePaneStopsAndCleansUpThroughTheRunner(t *testing.T) {
	h, fake, _ := startOnRunner(t, "headless")
	res, err := h.Server.RecoveryDo(recovery.Call{Verb: recovery.VerbClosePane, Address: "proj:epic-a/01"})
	if err != nil || res.Refused {
		t.Fatalf("close-pane = %+v, %v", res, err)
	}
	if _, found, _ := fake.Find(iterLabel); found {
		t.Error("session still live after close-pane")
	}
	if got := cleanedBy(fake); !slices.Equal(got, []string{iterLabel}) {
		t.Errorf("cleaned %v, want %s", got, iterLabel)
	}
}

func TestRecoveryRunner_WaitWaitsForTheRunnerToSettle(t *testing.T) {
	h, fake, _ := startOnRunner(t, "headless")
	fake.SetState(iterLabel, agentrunner.StateWorking, "")
	done := make(chan error, 1)
	go func() {
		res, err := h.Server.RecoveryDo(recovery.Call{Verb: recovery.VerbWait, Address: "proj:epic-a/01"})
		if err == nil && res.Refused {
			err = errors.New("refused: " + res.Reason)
		}
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("wait returned %v while the agent was working", err)
	case <-time.After(100 * time.Millisecond):
	}
	fake.SetState(iterLabel, agentrunner.StateIdle, "")
	if err := <-done; err != nil {
		t.Errorf("wait = %v", err)
	}
}

func TestRecoveryRunner_WaitTimesOutAtTheConfiguredBound(t *testing.T) {
	h, fake, _ := startOnRunner(t, "headless", func(c *server.Config) { c.CompactRewaitTimeout = 50 * time.Millisecond })
	fake.SetState(iterLabel, agentrunner.StateWorking, "")
	if _, err := h.Server.RecoveryDo(recovery.Call{Verb: recovery.VerbWait, Address: "proj:epic-a/01"}); !strings.Contains(err.Error(), agentrunner.ErrTimeout.Error()) {
		t.Errorf("wait err = %v, want a timeout", err)
	}
}

func TestRecoveryRunner_NudgePromptsThroughTheRunnerAndWatchesItsKind(t *testing.T) {
	h, fake, sess := startOnRunner(t, "headless")
	if res, err := h.Client.TicketNudge(context.Background(), "proj:epic-a/01", "go on"); err != nil || res.Refused {
		t.Fatalf("nudge = %+v, %v", res, err)
	}
	if got := fake.Prompts(iterLabel); !slices.Equal(got, []string{"go on"}) {
		t.Errorf("prompts = %q, want the nudge", got)
	}
	run, ok := h.Server.RunOf("proj:epic-a/01")
	if !ok || run.Runner != "headless" || run.Session.ID != sess.ID {
		t.Errorf("watched run = %+v, %v; want headless on %s", run, ok, sess.ID)
	}
	fake.SetState(iterLabel, agentrunner.StateIdle, "") // ends the watch
	evs, err := os.ReadFile(filepath.Join(h.TicketStore, "proj", "epic-a", "run-log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(evs), `"type":"`+string(events.Nudged)+`"`) || strings.Contains(string(evs), `"pane":`) {
		t.Errorf("run log should hold a pane-less nudged event:\n%s", evs)
	}
}
