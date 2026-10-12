package herdrrunner

import (
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/herdr"
)

// These cover the herdr wiring; the nudge/retype decisions themselves are
// tested in claudeui.

// pollTimeoutErr matches IsPollTimeout's "timed out" substring check.
var pollTimeoutErr = errors.New("timed out waiting for state")

var graceMs = int(claudeui.DefaultNudgeCaps.Grace / time.Millisecond)

// fixedNow returns a clock that never advances, for tests where elapsed
// time isn't under test.
func fixedNow() func() time.Time {
	t := time.Now()
	return func() time.Time { return t }
}

// stepNow returns a clock that advances by each of steps on successive
// calls (the first call returns the base instant unmodified), for tests
// asserting how PromptWithNudge divides a deadline across its phases.
func stepNow(steps ...time.Duration) func() time.Time {
	t := time.Now()
	calls := 0
	return func() time.Time {
		if calls > 0 && calls-1 < len(steps) {
			t = t.Add(steps[calls-1])
		}
		calls++
		return t
	}
}

// changingRead returns a read stub that returns a distinct string on every
// call (an incrementing counter), simulating a pane whose text keeps
// changing between polls — the "typed but not yet submitted" shape
// PromptWithNudge's bare-Enter nudge is meant to recover from.
func changingRead() func(target string, opts herdr.AgentReadOptions) (string, error) {
	calls := 0
	return func(target string, opts herdr.AgentReadOptions) (string, error) {
		calls++
		return strconv.Itoa(calls), nil
	}
}

func TestPromptWithNudge_SucceedsWithoutTimeout_NeverNudges(t *testing.T) {
	t.Parallel()
	var promptCalls, sendKeysCalls, waitCalls int
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptCalls++
		if opts.TimeoutMs != graceMs {
			t.Errorf("prompt TimeoutMs = %d, want %d", opts.TimeoutMs, graceMs)
		}
		return herdr.Agent{AgentStatus: "working"}, nil
	}
	sendKeys := func(target string, keys ...string) error {
		sendKeysCalls++
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitCalls++
		return herdr.Agent{}, nil
	}

	agent, err := promptWithNudge(prompt, sendKeys, wait, changingRead(), fixedNow())(herdr.AgentPromptOptions{
		Target: "pane-1",
		Text:   "hello",
		Wait:   true,
		Until:  []string{"working"},
	})
	if err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "working" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "working")
	}
	if promptCalls != 1 || sendKeysCalls != 0 || waitCalls != 0 {
		t.Errorf("promptCalls=%d sendKeysCalls=%d waitCalls=%d, want 1/0/0 (no nudge needed)", promptCalls, sendKeysCalls, waitCalls)
	}
}

func TestPromptWithNudge_TimesOutThenNudgeSucceeds(t *testing.T) {
	t.Parallel()
	var sendKeysTarget string
	var sendKeysKeys []string
	var waitOpts herdr.AgentWaitOptions

	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		return herdr.Agent{}, pollTimeoutErr
	}
	sendKeys := func(target string, keys ...string) error {
		sendKeysTarget = target
		sendKeysKeys = keys
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitOpts = opts
		return herdr.Agent{AgentStatus: "working"}, nil
	}

	// The pane's text changed since the initial submit (changingRead), so
	// this reads as "typed but not submitted" and should nudge, not retype.
	agent, err := promptWithNudge(prompt, sendKeys, wait, changingRead(), fixedNow())(herdr.AgentPromptOptions{
		Target: "pane-1",
		Text:   "hello",
		Wait:   true,
		Until:  []string{"working"},
	})
	if err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "working" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "working")
	}
	if sendKeysTarget != "pane-1" || len(sendKeysKeys) != 1 || sendKeysKeys[0] != "enter" {
		t.Errorf("sendKeys called with target=%q keys=%v, want pane-1/[enter]", sendKeysTarget, sendKeysKeys)
	}
	if waitOpts.Target != "pane-1" || len(waitOpts.Until) != 1 || waitOpts.Until[0] != "working" {
		t.Errorf("wait called with %+v, want Target=pane-1 Until=[working]", waitOpts)
	}
}

func TestPromptWithNudge_AgentPromptStalled_NudgeSucceeds(t *testing.T) {
	t.Parallel()
	stalledErr := errors.New(`$ herdr agent prompt w27:p7 ...

exit status 1

{"error":{"code":"agent_prompt_stalled","message":"agent prompt produced no observed state change within 5000 ms; status is idle and state_change_seq remained 946"}}`)
	var sendKeysCalls int
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		return herdr.Agent{}, stalledErr
	}
	sendKeys := func(target string, keys ...string) error {
		sendKeysCalls++
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		return herdr.Agent{AgentStatus: "working"}, nil
	}

	agent, err := promptWithNudge(prompt, sendKeys, wait, changingRead(), fixedNow())(herdr.AgentPromptOptions{
		Target: "pane-1",
		Text:   "hello",
		Wait:   true,
		Until:  []string{"working"},
	})
	if err != nil {
		t.Fatalf("PromptWithNudge() error = %v, want nudge to recover from agent_prompt_stalled", err)
	}
	if agent.AgentStatus != "working" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "working")
	}
	if sendKeysCalls != 1 {
		t.Errorf("sendKeysCalls = %d, want 1 (nudge should fire for agent_prompt_stalled)", sendKeysCalls)
	}
}

func TestPromptWithNudge_ExhaustsNudges_ReturnsTimeoutError(t *testing.T) {
	t.Parallel()
	var promptCalls, sendKeysCalls, waitCalls int
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptCalls++
		return herdr.Agent{}, pollTimeoutErr
	}
	sendKeys := func(target string, keys ...string) error {
		sendKeysCalls++
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitCalls++
		return herdr.Agent{}, pollTimeoutErr
	}

	_, err := promptWithNudge(prompt, sendKeys, wait, changingRead(), fixedNow())(herdr.AgentPromptOptions{
		Target: "pane-1",
		Text:   "hello",
		Wait:   true,
		Until:  []string{"working"},
	})
	if !IsPollTimeout(err) {
		t.Fatalf("PromptWithNudge() error = %v, want a poll-timeout error", err)
	}
	if errors.Is(err, claudeui.ErrStuckSubmission) {
		t.Errorf("PromptWithNudge() error = %v, want a plain poll-timeout error, not ErrStuckSubmission (the pane kept changing)", err)
	}
	if promptCalls != 1 {
		t.Errorf("promptCalls = %d, want 1 (only the initial submission)", promptCalls)
	}
	if sendKeysCalls != claudeui.DefaultNudgeCaps.Nudges || waitCalls != claudeui.DefaultNudgeCaps.Nudges {
		t.Errorf("sendKeysCalls=%d waitCalls=%d, want both = claudeui.DefaultNudgeCaps.Nudges (%d)", sendKeysCalls, waitCalls, claudeui.DefaultNudgeCaps.Nudges)
	}
}

func TestPromptWithNudge_UnchangedPane_ExhaustsRetypes_ReturnsStuckSubmission(t *testing.T) {
	t.Parallel()
	var promptCalls, sendKeysCalls int
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptCalls++
		if opts.Text != "hello" || opts.Target != "pane-1" {
			t.Errorf("prompt Target=%q Text=%q, want pane-1/hello (a retype resubmits the full text)", opts.Target, opts.Text)
		}
		return herdr.Agent{}, pollTimeoutErr
	}
	read := func(target string, opts herdr.AgentReadOptions) (string, error) {
		if target != "pane-1" || opts.Source != "recent-unwrapped" {
			t.Errorf("read(%q, %+v), want pane-1 with Source=recent-unwrapped", target, opts)
		}
		return "empty pane", nil
	}
	sendKeys := func(target string, keys ...string) error {
		sendKeysCalls++
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		t.Fatal("wait should not be called: the pane never changed across every retry")
		return herdr.Agent{}, nil
	}

	_, err := promptWithNudge(prompt, sendKeys, wait, read, fixedNow())(herdr.AgentPromptOptions{
		Target: "pane-1",
		Text:   "hello",
		Wait:   true,
		Until:  []string{"working"},
	})
	if !errors.Is(err, claudeui.ErrStuckSubmission) || !errors.Is(err, pollTimeoutErr) {
		t.Fatalf("PromptWithNudge() error = %v, want it to wrap ErrStuckSubmission and herdr's poll error", err)
	}
	// 1 initial submit + claudeui.DefaultNudgeCaps.Retypes retypes, then it gives up rather
	// than spending the rest of claudeui.DefaultNudgeCaps.Nudges on pointless Enter nudges.
	if promptCalls != 1+claudeui.DefaultNudgeCaps.Retypes {
		t.Errorf("promptCalls = %d, want %d (1 initial + claudeui.DefaultNudgeCaps.Retypes retypes)", promptCalls, 1+claudeui.DefaultNudgeCaps.Retypes)
	}
	if sendKeysCalls != 0 {
		t.Errorf("sendKeysCalls = %d, want 0", sendKeysCalls)
	}
}

func TestPromptWithNudge_FastCompletionBeforeWorking_ReturnsSuccessImmediately(t *testing.T) {
	t.Parallel()
	var promptCalls, sendKeysCalls, waitCalls int
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptCalls++
		if len(opts.Until) != 3 || opts.Until[0] != "working" || opts.Until[1] != "idle" || opts.Until[2] != "done" {
			t.Errorf("prompt Until = %v, want [working idle done] (start detection accepts either)", opts.Until)
		}
		return herdr.Agent{AgentStatus: "done"}, nil
	}
	sendKeys := func(target string, keys ...string) error {
		sendKeysCalls++
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitCalls++
		return herdr.Agent{}, nil
	}

	agent, err := promptWithNudge(prompt, sendKeys, wait, changingRead(), fixedNow())(herdr.AgentPromptOptions{
		Target:    "pane-1",
		Text:      "hello",
		Wait:      true,
		Until:     []string{"idle", "done"},
		TimeoutMs: 300_000,
	})
	if err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "done" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "done")
	}
	if promptCalls != 1 || sendKeysCalls != 0 || waitCalls != 0 {
		t.Errorf("promptCalls=%d sendKeysCalls=%d waitCalls=%d, want 1/0/0 (a final state observed before working returns immediately)", promptCalls, sendKeysCalls, waitCalls)
	}
}

func TestPromptWithNudge_StartConfirmed_WaitsForCompletionWithCallersTimeout(t *testing.T) {
	t.Parallel()
	var promptCalls int
	var completionWaitOpts herdr.AgentWaitOptions
	var completionWaitCalls int
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptCalls++
		if opts.TimeoutMs != graceMs {
			t.Errorf("prompt TimeoutMs = %d, want the short grace window %d, not the caller's completion timeout", opts.TimeoutMs, graceMs)
		}
		return herdr.Agent{AgentStatus: "working"}, nil
	}
	sendKeys := func(target string, keys ...string) error {
		t.Fatal("sendKeys should not be called: start was observed within the grace window")
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		completionWaitCalls++
		completionWaitOpts = opts
		return herdr.Agent{AgentStatus: "done"}, nil
	}

	agent, err := promptWithNudge(prompt, sendKeys, wait, changingRead(), fixedNow())(herdr.AgentPromptOptions{
		Target:    "pane-1",
		Text:      "/compact",
		Wait:      true,
		Until:     []string{"idle", "done"},
		TimeoutMs: 300_000,
	})
	if err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "done" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "done")
	}
	if promptCalls != 1 {
		t.Errorf("promptCalls = %d, want 1 (the prompt is submitted exactly once)", promptCalls)
	}
	if completionWaitCalls != 1 {
		t.Fatalf("completion wait calls = %d, want 1", completionWaitCalls)
	}
	if completionWaitOpts.TimeoutMs != 300_000 {
		t.Errorf("completion wait TimeoutMs = %d, want the caller's own 300000ms, not clobbered by the start-detection grace window", completionWaitOpts.TimeoutMs)
	}
	if len(completionWaitOpts.Until) != 2 || completionWaitOpts.Until[0] != "idle" || completionWaitOpts.Until[1] != "done" {
		t.Errorf("completion wait Until = %v, want [idle done]", completionWaitOpts.Until)
	}
}

func TestPromptWithNudge_SlowStart_CompletionGetsRemainingBudget(t *testing.T) {
	t.Parallel()
	promptAttempts := 0
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptAttempts++
		return herdr.Agent{}, pollTimeoutErr
	}
	sendKeys := func(target string, keys ...string) error {
		return nil
	}
	var completionWaitOpts herdr.AgentWaitOptions
	var completionWaitCalls, nudgeWaitCalls int
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if slices.Contains(opts.Until, "working") {
			nudgeWaitCalls++
			return herdr.Agent{AgentStatus: "working"}, nil
		}
		completionWaitCalls++
		completionWaitOpts = opts
		return herdr.Agent{AgentStatus: "done"}, nil
	}

	// 3s elapses between the deadline being captured and the first
	// start/nudge attempt (simulating a slow submit); the clock holds
	// steady after that.
	agent, err := promptWithNudge(prompt, sendKeys, wait, changingRead(), stepNow(3_000*time.Millisecond))(herdr.AgentPromptOptions{
		Target:    "pane-1",
		Text:      "/compact",
		Wait:      true,
		Until:     []string{"idle", "done"},
		TimeoutMs: 10_000,
	})
	if err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "done" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "done")
	}
	if promptAttempts != 1 {
		t.Errorf("promptAttempts = %d, want 1 (the prompt is submitted exactly once)", promptAttempts)
	}
	if nudgeWaitCalls != 1 {
		t.Errorf("nudgeWaitCalls = %d, want 1", nudgeWaitCalls)
	}
	if completionWaitCalls != 1 {
		t.Fatalf("completion wait calls = %d, want 1", completionWaitCalls)
	}
	if completionWaitOpts.TimeoutMs != 7_000 {
		t.Errorf("completion wait TimeoutMs = %d, want 7000 (the original 10000ms minus the 3000ms already spent on start)", completionWaitOpts.TimeoutMs)
	}
}

func TestPromptWithNudge_CompletionDeadlineAlreadyExpired_ReturnsTimeoutWithoutWaiting(t *testing.T) {
	t.Parallel()
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		return herdr.Agent{AgentStatus: "working"}, nil
	}
	sendKeys := func(target string, keys ...string) error {
		t.Fatal("sendKeys should not be called: start succeeded on the first attempt")
		return nil
	}
	waitCalls := 0
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitCalls++
		return herdr.Agent{}, nil
	}

	// 1s elapses before the start attempt, then a further 5s before the
	// completion-budget check, exhausting the caller's 5s deadline.
	_, err := promptWithNudge(prompt, sendKeys, wait, changingRead(), stepNow(1_000*time.Millisecond, 5_000*time.Millisecond))(herdr.AgentPromptOptions{
		Target:    "pane-1",
		Text:      "/compact",
		Wait:      true,
		Until:     []string{"idle", "done"},
		TimeoutMs: 5_000,
	})
	if !IsPollTimeout(err) {
		t.Fatalf("PromptWithNudge() error = %v, want a poll-timeout error reporting the overall deadline", err)
	}
	if waitCalls != 0 {
		t.Errorf("waitCalls = %d, want 0 (an already-expired deadline must not wait, unbounded or otherwise)", waitCalls)
	}
}

func TestPromptWithNudge_ZeroTimeout_BoundedStartThenUnlimitedCompletion(t *testing.T) {
	t.Parallel()
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if opts.TimeoutMs != graceMs {
			t.Errorf("prompt TimeoutMs = %d, want the bounded grace window %d even with no caller deadline", opts.TimeoutMs, graceMs)
		}
		return herdr.Agent{AgentStatus: "working"}, nil
	}
	sendKeys := func(target string, keys ...string) error {
		return nil
	}
	var completionWaitOpts herdr.AgentWaitOptions
	completionWaitCalls := 0
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		completionWaitCalls++
		completionWaitOpts = opts
		return herdr.Agent{AgentStatus: "done"}, nil
	}

	agent, err := promptWithNudge(prompt, sendKeys, wait, changingRead(), fixedNow())(herdr.AgentPromptOptions{
		Target:    "pane-1",
		Text:      "/compact",
		Wait:      true,
		Until:     []string{"idle", "done"},
		TimeoutMs: 0,
	})
	if err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "done" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "done")
	}
	if completionWaitCalls != 1 {
		t.Fatalf("completion wait calls = %d, want 1", completionWaitCalls)
	}
	if completionWaitOpts.TimeoutMs != 0 {
		t.Errorf("completion wait TimeoutMs = %d, want 0 (unlimited: a zero caller timeout means no deadline)", completionWaitOpts.TimeoutMs)
	}
}
