package ralphloop

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// An agent that ends its turn with a backgrounded test run still going reads as
// idle. The server must keep waiting, or it parks the ticket zero-commit before
// the agent commits.
func TestWaitIterationFinished_HoldsWhileBackgroundTaskRuns(t *testing.T) {
	t.Parallel()
	var sleeps, reads int
	d := idleBackgroundTaskDeps(func(string, string) (transcript.BackgroundTaskReading, error) {
		reads++
		status := transcript.BackgroundTaskOutstandingFresh
		if reads >= 3 {
			status = transcript.BackgroundTaskResolved
		}
		return transcript.BackgroundTaskReading{Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: status}}}, nil
	}, &sleeps)
	d.AgentGet = func(string) (herdr.Agent, error) { return herdr.Agent{AgentSession: "sess-1"}, nil }

	one := OneIteration{Agent: AgentClaude}
	wt := IterationWorktree{Label: "iter-27", Path: "/repo/iter-27"}
	if err := WaitIterationFinished(d, one, wt, "pane-1"); err != nil {
		t.Fatalf("WaitIterationFinished: %v", err)
	}
	if reads != 3 {
		t.Errorf("ReadBackgroundTasks calls = %d, want 3 (held, held, resolved)", reads)
	}
}

// A first idle that the debounce disproves (the agent went back to work) must
// not end the wait.
func TestWaitIterationFinished_TransientIdleKeepsWaiting(t *testing.T) {
	t.Parallel()
	var waits int
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			// 1st: idle. 2nd (the debounce re-poll): still working, so it times out.
			// 3rd: idle again. 4th: the re-poll confirms.
			if waits == 2 {
				return herdr.Agent{}, errors.New("timed out")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentGet: func(string) (herdr.Agent, error) { return herdr.Agent{}, nil },
		Sleep:    func(time.Duration) {},
	}
	one := OneIteration{Agent: AgentClaude}
	if err := WaitIterationFinished(d, one, IterationWorktree{Label: "iter-27"}, "pane-1"); err != nil {
		t.Fatalf("WaitIterationFinished: %v", err)
	}
	if waits != 4 {
		t.Errorf("AgentWait calls = %d, want 4 (idle, blip, idle, confirmed)", waits)
	}
}

// A held land lock is routine; landBuilt retries instead of failing the finish.
func TestLandBuilt_RetriesWhileLandLockHeld(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	lockDir := mustLandLockDir(t, scratchDir, "epic")
	if err := AcquireLandLock(lockDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ReleaseLandLock(lockDir) })
	d, _, _ := fakeDeps()
	var sleeps int
	d.Sleep = func(time.Duration) {
		sleeps++
		if sleeps == 3 {
			_ = ReleaseLandLock(lockDir)
		}
	}
	p := iterationParams{FeatureBranch: "epic", ScratchDir: scratchDir, WorktreeLock: &worktreeLock, Gate: NewGate(), Sink: noopEventSink{}}
	built := &builtAwaitingLandError{job: landJob{ticket: tickets.Ticket{Identifier: "01"}}}

	err := landBuilt(d, p, built)
	if err != nil && strings.Contains(err.Error(), "land deferred") {
		t.Fatalf("landBuilt gave up on a held lock: %v", err)
	}
	if sleeps != 3 {
		t.Errorf("retry sleeps = %d, want 3 (held 3 times, then the lock was free)", sleeps)
	}
}
