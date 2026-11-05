package fakes

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/transcript"
)

// BlipRunner times out its TimeoutOn-th Wait: a finish recheck finds the
// agent back at work. runnerfake can't script that without a real-time
// timeout.
type BlipRunner struct {
	*runnerfake.Runner
	TimeoutOn int
	// TimeoutIf, when set, replaces TimeoutOn: it is told each Wait's 1-based
	// index and reports whether that Wait times out.
	TimeoutIf func(wait int) bool
	// TimeoutLabel, when set, also times out every Wait on a label it matches.
	TimeoutLabel func(label string) bool
	// FinishPollsOnly scripts only supervise.WaitForFinish's own polls, told apart by
	// also stopping on blocked: TimeoutOn and TimeoutIf index those alone, and
	// every other Wait, such as recovery's compact polls, sees the fake's state.
	FinishPollsOnly bool
	// InterruptErr, when set, fails every Interrupt.
	InterruptErr error
	// PromptErr, when set, fails every Prompt without recording it.
	PromptErr error
	// ContextExhausted, when set, is the evidence of the next RateLimit call
	// alone failing with ErrContextExhausted; later calls see the fake's state.
	ContextExhausted string
	Waits            int
	FinishPolls      int
	Interrupts       int
}

func (r *BlipRunner) RateLimit(s agentrunner.Session) (time.Time, bool, error) {
	if evidence := r.ContextExhausted; evidence != "" {
		r.ContextExhausted = ""
		return time.Time{}, false, fmt.Errorf("%w: %s", agentrunner.ErrContextExhausted, evidence)
	}
	return r.Runner.RateLimit(s)
}

func (r *BlipRunner) Interrupt(s agentrunner.Session) error {
	r.Interrupts++
	if r.InterruptErr != nil {
		return r.InterruptErr
	}
	return r.Runner.Interrupt(s)
}

func (r *BlipRunner) Prompt(s agentrunner.Session, text string) error {
	if r.PromptErr != nil {
		return r.PromptErr
	}
	return r.Runner.Prompt(s, text)
}

func (r *BlipRunner) Wait(s agentrunner.Session, states []agentrunner.State, timeout time.Duration) (agentrunner.Status, error) {
	r.Waits++
	n := r.Waits
	if r.FinishPollsOnly {
		if !slices.Contains(states, agentrunner.StateBlocked) {
			return r.Runner.Wait(s, states, timeout)
		}
		r.FinishPolls++
		n = r.FinishPolls
	}
	timedOut := n == r.TimeoutOn
	if r.TimeoutIf != nil {
		timedOut = r.TimeoutIf(n)
	}
	if r.TimeoutLabel != nil && r.TimeoutLabel(s.Label) {
		timedOut = true
	}
	if timedOut {
		return agentrunner.Status{State: agentrunner.StateWorking}, agentrunner.ErrTimeout
	}
	return r.Runner.Wait(s, states, timeout)
}

// IdlePromptRunner is PaneRunner on pane-1 whose agent reports idle the moment
// it is prompted, so a wait after "/compact" never blocks in real time.
func IdlePromptRunner(label, sessionID string) *runnerfake.Runner {
	r := PaneRunner(label, "pane-1", sessionID)
	r.PromptState = agentrunner.StateIdle
	return r
}

// BoundGiveUpRunner hosts the give-up-bound tests' iter-19 session; its finish
// polls time out wherever timeoutIf says. Recovery's own compact-completion
// polls see the agent idle right after "/compact": the premature idle the gate
// exists to distrust.
func BoundGiveUpRunner(timeoutIf func(wait int) bool) *BlipRunner {
	return &BlipRunner{Runner: IdlePromptRunner("iter-19", "sess-19"), TimeoutIf: timeoutIf, FinishPollsOnly: true}
}

// VerifyCapFixture is the outcome of a wait run on a pane whose first polls
// time out, for the verify-cap tests.
type VerifyCapFixture struct {
	R          *BlipRunner
	ScratchDir string
	Err        error
}

// IdleBackgroundTaskDeps is the base Deps for WaitForBackgroundTasks tests: a
// pane that reports idle on every AgentWait (including ConfirmFinished's own
// re-check), so readBackgroundTasks is the only thing that can hold
// WaitForFinish's conclusion open.
func IdleBackgroundTaskDeps(readBackgroundTasks func(cwd, sessionID string) (transcript.BackgroundTaskReading, error), sleeps *int) deps.Deps {
	return deps.Deps{
		Runner:              IdleRunner("iter-30"),
		ReadBackgroundTasks: readBackgroundTasks,
		Sleep:               func(time.Duration) { *sleeps++ },
	}
}

// OccupancySink is a minimal EventSink test double that only records
// ContextOccupancy calls, embedding runlog.NoopEventSink for the rest.
type OccupancySink struct {
	runlog.NoopEventSink
	mu    sync.Mutex
	Calls []OccupancyCall
}

type OccupancyCall struct {
	Identifier string
	Tokens     int
}

func (s *OccupancySink) ContextOccupancy(identifier string, tokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls = append(s.Calls, OccupancyCall{Identifier: identifier, Tokens: tokens})
}

// CapStopEvents counts the verify-cap-stopped events in scratchDir's epic log.
func CapStopEvents(t *testing.T, scratchDir string) int {
	t.Helper()
	evs, _, err := runlog.ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ev := range evs {
		if ev.Type == string(events.VerifyCapStopped) {
			n++
		}
	}
	return n
}
