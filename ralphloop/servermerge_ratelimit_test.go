package ralphloop

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/testutil/runnerfake"
)

// promptHookRunner runs onPrompt after each delivered prompt, with the count so far.
type promptHookRunner struct {
	*runnerfake.Runner
	onPrompt func(n int)
	n        int
}

func (r *promptHookRunner) Prompt(s agentrunner.Session, text string) error {
	err := r.Runner.Prompt(s, text)
	if err == nil {
		r.n++
		r.onPrompt(r.n)
	}
	return err
}

var rateLimitClock = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// rateLimitFixture is an idle, rate-limited agent with a finished report (so the
// stall nudge cannot fire) on a fake clock that Sleep advances.
func rateLimitFixture(t *testing.T, limit func(r *runnerfake.Runner), onPrompt func(r *runnerfake.Runner, n int)) (Deps, *runnerfake.Runner, OneIteration, *[]time.Duration, *[]bool) {
	t.Helper()
	d, r, one, sleeps := stallNudgeFixture(t, "finished", 2*time.Minute)
	*sleeps = nil
	now := rateLimitClock
	d.Now = func() time.Time { return now }
	d.Sleep = func(dur time.Duration) {
		*sleeps = append(*sleeps, dur)
		now = now.Add(dur)
	}
	d.Runner = &promptHookRunner{Runner: r, onPrompt: func(n int) { onPrompt(r, n) }}
	limit(r)
	var holds []bool
	one.OnRateLimit = func(limited bool, _ time.Time) { holds = append(holds, limited) }
	return d, r, one, sleeps, &holds
}

func eventsOfType(t *testing.T, one OneIteration, typ events.Type) int {
	t.Helper()
	evs, _, err := ReadEvents(one.ScratchDir, one.Epic)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ev := range evs {
		if ev.Type == string(typ) {
			n++
		}
	}
	return n
}

// holdSleeps returns the first n sleeps; the finish path sleeps after the hold.
func holdSleeps(sleeps []time.Duration, n int) []time.Duration {
	if len(sleeps) < n {
		return sleeps
	}
	return sleeps[:n]
}

func clearOn(n int) func(r *runnerfake.Runner, got int) {
	return func(r *runnerfake.Runner, got int) {
		if got >= n {
			r.SetRateLimit("iter-01", time.Time{})
		}
	}
}

func TestWaitIterationFinished_RateLimitHoldsUntilResetPlusBuffer(t *testing.T) {
	d, r, one, sleeps, holds := rateLimitFixture(t, func(r *runnerfake.Runner) {
		r.SetRateLimit("iter-01", rateLimitClock.Add(30*time.Second))
	}, clearOn(1))
	waitStallFixture(t, d, one)
	if got := r.Prompts("iter-01"); !slices.Equal(got, []string{"continue"}) {
		t.Errorf("prompts = %q, want one continue and no stall nudge", got)
	}
	if want := []time.Duration{30*time.Second + rateLimitResetBuffer}; !slices.Equal(holdSleeps(*sleeps, 1), want) {
		t.Errorf("sleeps = %v, want %v first", *sleeps, want)
	}
	if !slices.Equal(*holds, []bool{true, false}) {
		t.Errorf("OnRateLimit calls = %v, want hold then release", *holds)
	}
	if n := eventsOfType(t, one, events.PausedRateLimit); n != 1 {
		t.Errorf("paused-rate-limit events = %d, want 1", n)
	}
	if n := eventsOfType(t, one, events.Resumed); n != 1 {
		t.Errorf("resumed events = %d, want 1", n)
	}
}

func TestWaitIterationFinished_RateLimitPastResetContinuesAtOnce(t *testing.T) {
	d, r, one, sleeps, _ := rateLimitFixture(t, func(r *runnerfake.Runner) {
		r.SetRateLimit("iter-01", rateLimitClock.Add(-time.Hour))
	}, clearOn(1))
	waitStallFixture(t, d, one)
	if got := r.Prompts("iter-01"); !slices.Equal(got, []string{"continue"}) {
		t.Errorf("prompts = %q, want one continue", got)
	}
	if slices.Contains(*sleeps, rateLimitResetBuffer) || slices.Contains(*sleeps, rateLimitPollInterval) {
		t.Errorf("sleeps = %v, want no hold sleep", *sleeps)
	}
}

func TestWaitIterationFinished_RateLimitUnknownResetPollsWhileLimitedAgain(t *testing.T) {
	d, r, one, sleeps, _ := rateLimitFixture(t, func(r *runnerfake.Runner) {
		r.SetLimitedUnknownReset("iter-01")
	}, clearOn(2))
	waitStallFixture(t, d, one)
	if got := r.Prompts("iter-01"); !slices.Equal(got, []string{"continue", "continue"}) {
		t.Errorf("prompts = %q, want two continues", got)
	}
	if want := []time.Duration{rateLimitPollInterval, rateLimitPollInterval}; !slices.Equal(holdSleeps(*sleeps, 2), want) {
		t.Errorf("sleeps = %v, want %v first", *sleeps, want)
	}
}

func TestWaitIterationFinished_RateLimitedBlockedAgentHolds(t *testing.T) {
	d, r, one, sleeps, _ := rateLimitFixture(t, func(r *runnerfake.Runner) {
		r.SetState("iter-01", agentrunner.StateBlocked, "usage limit")
		r.SetRateLimit("iter-01", rateLimitClock.Add(time.Minute))
	}, func(*runnerfake.Runner, int) {})
	one.Agent, one.Runner = AgentClaude, string(agentrunner.ChoiceHeadless)
	d.Sleep = func(dur time.Duration) {
		*sleeps = append(*sleeps, dur)
		r.SetState("iter-01", agentrunner.StateIdle, "")
		r.SetRateLimit("iter-01", time.Time{})
	}
	err := waitBlocked(t, d, one)
	var paused *BlockedPausedError
	if errors.As(err, &paused) || err != nil {
		t.Fatalf("err = %v, want nil: a limited block holds, it does not pause", err)
	}
	if len(*sleeps) == 0 || (*sleeps)[0] != time.Minute+rateLimitResetBuffer {
		t.Errorf("sleeps = %v, want the hold sleep first", *sleeps)
	}
}
