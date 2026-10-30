package ralphloop

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

// An agent that ends its turn with a backgrounded test run still going reads as
// idle. The server must keep waiting, or it pauses the ticket zero-commit before
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

// stallNudgeFixture is an idle agent whose ticket carries iterationStatus (""
// for none) and the OneIteration that waits on it.
func stallNudgeFixture(t *testing.T, iterationStatus string, delay time.Duration) (Deps, *runnerfake.Runner, OneIteration, *[]time.Duration) {
	t.Helper()
	front := "---\nid: \"01\"\nstatus: claimed\ntype: implement\n"
	if iterationStatus != "" {
		front += "iteration_status: " + iterationStatus + "\n"
	}
	scratchDir := writeEpic(t, "epic", map[string]string{"01-a.md": front + "---\n# A\n"})
	r := idleRunner("iter-01")
	r.PromptState = agentrunner.StateIdle
	var sleeps []time.Duration
	d := Deps{Runner: r, Sleep: func(d time.Duration) { sleeps = append(sleeps, d) }}
	one := OneIteration{
		Agent: AgentClaude, Epic: "epic", ScratchDir: scratchDir, StallNudgeDelay: delay,
		Ticket: tickets.Ticket{Identifier: "01", Path: filepath.Join(scratchDir, "epic", "issues", "01-a.md")},
	}
	return d, r, one, &sleeps
}

func waitStallFixture(t *testing.T, d Deps, one OneIteration) {
	t.Helper()
	if err := WaitIterationFinished(d, one, IterationWorktree{Label: "iter-01"}, agentrunner.Session{Label: "iter-01", ID: "pane-1"}); err != nil {
		t.Fatalf("WaitIterationFinished: %v", err)
	}
}

func stallNudgeEvents(t *testing.T, one OneIteration) []Event {
	t.Helper()
	evs, _, err := ReadEvents(one.ScratchDir, one.Epic)
	if err != nil {
		t.Fatal(err)
	}
	var out []Event
	for _, ev := range evs {
		if ev.Type == string(events.StallNudged) {
			out = append(out, ev)
		}
	}
	return out
}

// An idle agent with no question and no finished report gets one nudge after
// the delay, then the wait ends and the existing finish path applies.
func TestWaitIterationFinished_NudgesIdleAgentWithNoReportOnce(t *testing.T) {
	t.Parallel()
	d, r, one, sleeps := stallNudgeFixture(t, "", 2*time.Minute)
	waitStallFixture(t, d, one)
	if got := r.Prompts("iter-01"); len(got) != 1 || got[0] != StallNudgeText {
		t.Errorf("prompts = %q, want exactly the stall nudge", got)
	}
	if !slices.Contains(*sleeps, 2*time.Minute) {
		t.Errorf("sleeps = %v, want the 2m delay", *sleeps)
	}
	if evs := stallNudgeEvents(t, one); len(evs) != 1 || evs[0].Text != StallNudgeText || evs[0].Ticket != "01" {
		t.Errorf("run log stall-nudged events = %+v, want one with the nudge text", evs)
	}
}

func TestWaitIterationFinished_NoNudgeWhileQuestionOpenOrFinished(t *testing.T) {
	t.Parallel()
	for _, status := range []string{string(schema.IterationStatusAsking), "finished"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			d, r, one, sleeps := stallNudgeFixture(t, status, 2*time.Minute)
			waitStallFixture(t, d, one)
			if got := r.Prompts("iter-01"); len(got) != 0 {
				t.Errorf("prompts = %q, want none", got)
			}
			if slices.Contains(*sleeps, 2*time.Minute) {
				t.Errorf("sleeps = %v, want no stall delay", *sleeps)
			}
			if evs := stallNudgeEvents(t, one); len(evs) != 0 {
				t.Errorf("stall-nudged events = %+v, want none", evs)
			}
		})
	}
}

func TestWaitIterationFinished_ZeroDelayDisablesNudge(t *testing.T) {
	t.Parallel()
	d, r, one, _ := stallNudgeFixture(t, "", 0)
	waitStallFixture(t, d, one)
	if got := r.Prompts("iter-01"); len(got) != 0 {
		t.Errorf("prompts = %q, want none", got)
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
	// timeoutLabel, when set, also times out every Wait on a label it matches.
	timeoutLabel func(label string) bool
	// finishPollsOnly scripts only waitForFinish's own polls, told apart by
	// also stopping on blocked: timeoutOn and timeoutIf index those alone, and
	// every other Wait, such as recovery's compact polls, sees the fake's state.
	finishPollsOnly bool
	// interruptErr, when set, fails every Interrupt.
	interruptErr error
	// promptErr, when set, fails every Prompt without recording it.
	promptErr error
	// contextExhausted, when set, is the evidence of the next RateLimit call
	// alone failing with ErrContextExhausted; later calls see the fake's state.
	contextExhausted string
	waits            int
	finishPolls      int
	interrupts       int
}

func (r *blipRunner) RateLimit(s agentrunner.Session) (time.Time, bool, error) {
	if evidence := r.contextExhausted; evidence != "" {
		r.contextExhausted = ""
		return time.Time{}, false, fmt.Errorf("%w: %s", agentrunner.ErrContextExhausted, evidence)
	}
	return r.Runner.RateLimit(s)
}

func (r *blipRunner) Interrupt(s agentrunner.Session) error {
	r.interrupts++
	if r.interruptErr != nil {
		return r.interruptErr
	}
	return r.Runner.Interrupt(s)
}

func (r *blipRunner) Prompt(s agentrunner.Session, text string) error {
	if r.promptErr != nil {
		return r.promptErr
	}
	return r.Runner.Prompt(s, text)
}

func (r *blipRunner) Wait(s agentrunner.Session, states []agentrunner.State, timeout time.Duration) (agentrunner.Status, error) {
	r.waits++
	n := r.waits
	if r.finishPollsOnly {
		if !slices.Contains(states, agentrunner.StateBlocked) {
			return r.Runner.Wait(s, states, timeout)
		}
		r.finishPolls++
		n = r.finishPolls
	}
	timedOut := n == r.timeoutOn
	if r.timeoutIf != nil {
		timedOut = r.timeoutIf(n)
	}
	if r.timeoutLabel != nil && r.timeoutLabel(s.Label) {
		timedOut = true
	}
	if timedOut {
		return agentrunner.Status{State: agentrunner.StateWorking}, agentrunner.ErrTimeout
	}
	return r.Runner.Wait(s, states, timeout)
}

// A held merge lock is routine; mergeBuilt retries instead of failing the finish.
func TestMergeBuilt_RetriesWhileMergeLockHeld(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	lockDir := mustMergeLockDir(t, scratchDir, "epic")
	if err := AcquireMergeLock(lockDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ReleaseMergeLock(lockDir) })
	d, _, _ := fakeDeps()
	var sleeps int
	d.Sleep = func(time.Duration) {
		sleeps++
		if sleeps == 3 {
			_ = ReleaseMergeLock(lockDir)
		}
	}
	p := iterationParams{FeatureBranch: "epic", ScratchDir: scratchDir, WorktreeLock: &worktreeLock, Gate: NewGate(), Sink: noopEventSink{}}
	built := &builtAwaitingMergeError{job: mergeJob{ticket: tickets.Ticket{Identifier: "01"}}}

	err := mergeBuilt(d, p, built)
	if err != nil && strings.Contains(err.Error(), "merge deferred") {
		t.Fatalf("mergeBuilt gave up on a held lock: %v", err)
	}
	if sleeps != 3 {
		t.Errorf("retry sleeps = %d, want 3 (held 3 times, then the lock was free)", sleeps)
	}
}

// blockedWaitFixture is a blocked agent on the named runner; onSleep runs when
// the loop sleeps (the dwell).
func blockedWaitFixture(t *testing.T, agent AgentKind, runner string, onSleep func(*runnerfake.Runner)) (Deps, *runnerfake.Runner, OneIteration, *[]time.Duration) {
	t.Helper()
	d, r, one, sleeps := stallNudgeFixture(t, "", 0)
	one.Agent, one.Runner = agent, runner
	r.SetState("iter-01", agentrunner.StateBlocked, "Bash: rm -rf x")
	d.Sleep = func(dur time.Duration) {
		*sleeps = append(*sleeps, dur)
		if onSleep != nil {
			onSleep(r)
		}
	}
	return d, r, one, sleeps
}

func waitBlocked(t *testing.T, d Deps, one OneIteration) error {
	t.Helper()
	return WaitIterationFinished(d, one, IterationWorktree{Label: "iter-01"}, agentrunner.Session{Label: "iter-01", ID: "pane-1"})
}

func TestWaitIterationFinished_HeadlessBlockedPausesAtOnce(t *testing.T) {
	d, _, one, sleeps := blockedWaitFixture(t, AgentClaude, string(agentrunner.ChoiceHeadless), nil)
	var paused *BlockedPausedError
	if err := waitBlocked(t, d, one); !errors.As(err, &paused) {
		t.Fatalf("err = %v, want *BlockedPausedError", err)
	}
	if len(*sleeps) != 0 {
		t.Errorf("sleeps = %v, want none", *sleeps)
	}
}

func TestWaitIterationFinished_DwellingRunnersPauseOnlyIfStillBlocked(t *testing.T) {
	cases := []struct {
		name   string
		agent  AgentKind
		runner string
		want   []string
	}{
		{"pty claude", AgentClaude, "pty", []string{"web answer button", "gx server agents answer", "Agents tab live panel"}},
		{"pty codex", AgentCodex, "pty", []string{"web answer button", "gx server agents answer", "Agents tab live panel"}},
		{"herdr claude", AgentClaude, "herdr", []string{"gx server agents answer", "or in the pane"}},
		{"herdr codex", AgentCodex, "herdr", []string{"gx server agents answer", "or in the pane"}},
	}
	for _, c := range cases {
		t.Run(c.name+" still blocked", func(t *testing.T) {
			d, _, one, sleeps := blockedWaitFixture(t, c.agent, c.runner, nil)
			var paused *BlockedPausedError
			if err := waitBlocked(t, d, one); !errors.As(err, &paused) {
				t.Fatalf("err = %v, want *BlockedPausedError", err)
			}
			if !slices.Equal(*sleeps, []time.Duration{blockedDwellMs * time.Millisecond}) {
				t.Errorf("sleeps = %v, want one dwell", *sleeps)
			}
			for _, w := range c.want {
				if !strings.Contains(paused.Reason, w) {
					t.Errorf("reason %q lacks %q", paused.Reason, w)
				}
			}
			if c.runner != "pty" && strings.Contains(paused.Reason, "web answer button") {
				t.Errorf("reason %q names the web button on %s", paused.Reason, c.runner)
			}
		})
		t.Run(c.name+" clears in the dwell", func(t *testing.T) {
			d, _, one, sleeps := blockedWaitFixture(t, c.agent, c.runner, func(r *runnerfake.Runner) {
				r.SetState("iter-01", agentrunner.StateIdle, "")
			})
			if err := waitBlocked(t, d, one); err != nil {
				t.Fatalf("err = %v, want the wait to finish", err)
			}
			if len(*sleeps) == 0 || (*sleeps)[0] != blockedDwellMs*time.Millisecond {
				t.Errorf("sleeps = %v, want the dwell first", *sleeps)
			}
		})
	}
}
