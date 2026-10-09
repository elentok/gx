package ralphloop

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/testutil/runnerfake"
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
	d.Runner = idleRunner("iter-27")

	one := OneIteration{Agent: AgentClaude}
	wt := IterationWorktree{Label: "iter-27", Path: "/repo/iter-27"}
	if err := WaitIterationFinished(d, one, wt, agentrunner.Session{Label: "iter-27", ID: "pane-1"}); err != nil {
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
	r := &blipRunner{Runner: idleRunner("iter-27"), timeoutOn: 2}
	d := Deps{Runner: r, Sleep: func(time.Duration) {}}
	one := OneIteration{Agent: AgentClaude}
	if err := WaitIterationFinished(d, one, IterationWorktree{Label: "iter-27"}, agentrunner.Session{Label: "iter-27", ID: "pane-1"}); err != nil {
		t.Fatalf("WaitIterationFinished: %v", err)
	}
	if r.waits != 4 {
		t.Errorf("Runner.Wait calls = %d, want 4 (idle, blip, idle, confirmed)", r.waits)
	}
}

// blipRunner times out its timeoutOn-th Wait: a finish recheck finds the
// agent back at work. runnerfake can't script that without a real-time
// timeout.
type blipRunner struct {
	*runnerfake.Runner
	timeoutOn int
	// timeoutIf, when set, replaces timeoutOn: it is told each Wait's 1-based
	// index and reports whether that Wait times out.
	timeoutIf func(wait int) bool
	waits     int
}

func (r *blipRunner) Wait(s agentrunner.Session, states []agentrunner.State, timeout time.Duration) (agentrunner.Status, error) {
	r.waits++
	timedOut := r.waits == r.timeoutOn
	if r.timeoutIf != nil {
		timedOut = r.timeoutIf(r.waits)
	}
	if timedOut {
		return agentrunner.Status{State: agentrunner.StateWorking}, agentrunner.ErrTimeout
	}
	return r.Runner.Wait(s, states, timeout)
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
