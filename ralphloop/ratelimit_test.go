package ralphloop

import (
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
)

func TestWaitForClaudeRateLimitReset_KnownReset_ReturnsOnceDeadlinePasses(t *testing.T) {
	t.Parallel()
	g := NewGate()
	g.pause("t1", "rate limit detected")

	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	current := base
	slept := 0
	d := Deps{
		Runner: idleRunner("t1"),
		Sleep: func(time.Duration) {
			slept++
			current = current.Add(rateLimitPollInterval)
		},
		Now: func() time.Time { return current },
	}

	waitForClaudeRateLimitReset(d, g, agentrunner.Session{Label: "t1", ID: "pane-1"}, base.Add(5*time.Minute))

	if slept == 0 {
		t.Errorf("Sleep never called, want at least one poll before the deadline check")
	}
}

func TestWaitForClaudeRateLimitReset_UnknownReset_PollsUntilRunnerReportsCleared(t *testing.T) {
	t.Parallel()
	g := NewGate()
	g.pause("t1", "rate limit detected")

	runner := idleRunner("t1")
	runner.SetLimitedUnknownReset("t1")
	current := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	polls := 0
	d := Deps{
		Runner: runner,
		// Advance wall-clock time on each Sleep so the coarse
		// rateLimitPollInterval recheck cadence is actually reached without a
		// real sleep.
		Sleep: func(time.Duration) {
			current = current.Add(rateLimitPollInterval)
			polls++
			if polls == 3 {
				runner.SetRateLimit("t1", time.Time{})
			}
		},
		Now: func() time.Time { return current },
	}

	waitForClaudeRateLimitReset(d, g, agentrunner.Session{Label: "t1", ID: "pane-1"}, time.Time{})

	if polls != 3 {
		t.Errorf("Sleep called %d times, want 3 (poll until the runner reports it cleared)", polls)
	}
}

func TestWaitForClaudeRateLimitReset_ForceResumed_ReturnsImmediately(t *testing.T) {
	t.Parallel()
	g := NewGate()
	g.pause("t1", "rate limit detected")
	g.ForceResume("t1")

	d := Deps{
		Sleep: func(time.Duration) { t.Fatal("should not sleep: already force-resumed") },
		Now:   time.Now,
	}

	waitForClaudeRateLimitReset(d, g, agentrunner.Session{Label: "t1", ID: "pane-1"}, time.Now().Add(time.Hour))
}

func TestWaitForCodexRateLimitReset_MissingResetPollsUntilQuotaClears(t *testing.T) {
	t.Parallel()
	runner := idleRunner("iter-01")
	runner.SetLimitedUnknownReset("iter-01")
	d := Deps{Runner: runner}
	var sleeps []time.Duration
	d.Sleep = func(duration time.Duration) {
		sleeps = append(sleeps, duration)
		if len(sleeps) == 2 {
			runner.SetRateLimit("iter-01", time.Time{})
		}
	}

	waitForCodexRateLimitReset(d, agentrunner.Session{Label: "iter-01", ID: "pane-1"}, time.Time{})

	if len(sleeps) != 2 || sleeps[0] != rateLimitPollInterval || sleeps[1] != rateLimitPollInterval {
		t.Errorf("sleeps = %v, want two %v polls", sleeps, rateLimitPollInterval)
	}
}

func TestWaitForCodexRateLimitReset_MissingResetAt_NeverClears_BoundedThenReturns(t *testing.T) {
	t.Parallel()
	runner := idleRunner("iter-01")
	runner.SetLimitedUnknownReset("iter-01")
	sleeps := 0
	d := Deps{Runner: runner, Sleep: func(time.Duration) { sleeps++ }}

	done := make(chan struct{})
	go func() {
		waitForCodexRateLimitReset(d, agentrunner.Session{Label: "iter-01", ID: "pane-1"}, time.Time{})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForCodexRateLimitReset never returned: an unchanging exhausted snapshot polled indefinitely")
	}

	if sleeps != codexRateLimitMaxRepolls {
		t.Errorf("quota rechecks = %d, want %d (bounded)", sleeps, codexRateLimitMaxRepolls)
	}
}

func TestWaitForCodexRateLimitReset_SleepsPastResetThenReobserves(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	runner := idleRunner("iter-01")
	runner.SetRateLimit("iter-01", base.Add(2*time.Second))
	d := Deps{Runner: runner, Now: func() time.Time { return base }}
	var sleeps []time.Duration
	d.Sleep = func(duration time.Duration) {
		sleeps = append(sleeps, duration)
		runner.SetRateLimit("iter-01", time.Time{})
	}

	waitForCodexRateLimitReset(d, agentrunner.Session{Label: "iter-01", ID: "pane-1"}, base.Add(2*time.Second))

	if len(sleeps) != 1 || sleeps[0] < rateLimitResetBuffer {
		t.Errorf("sleeps = %v, want a single sleep through the reset plus buffer", sleeps)
	}
}

func TestWaitForCodexRateLimitReset_StaleResetAt_NoWaitAndBoundedRepolls(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	stale := base.Add(-time.Hour)
	runner := idleRunner("iter-01")
	runner.SetRateLimit("iter-01", stale)
	d := Deps{Runner: runner, Now: func() time.Time { return base }}
	var sleeps []time.Duration
	d.Sleep = func(duration time.Duration) { sleeps = append(sleeps, duration) }

	// The reset is already in the past relative to the deterministic clock —
	// an immutable pre-reset record from a deadline that already passed.
	waitForCodexRateLimitReset(d, agentrunner.Session{Label: "iter-01", ID: "pane-1"}, stale)

	if len(sleeps) != codexRateLimitMaxRepolls {
		t.Errorf("sleeps = %v, want %d bounded repoll sleeps and no deadline wait", sleeps, codexRateLimitMaxRepolls)
	}
}

func TestWaitForCodexRateLimitReset_ClearedRightAfterDeadline_ReturnsWithoutRepolling(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	d := Deps{
		Runner: idleRunner("iter-01"),
		Now:    func() time.Time { return base },
		Sleep: func(duration time.Duration) {
			t.Errorf("slept %v: reset already passed and quota cleared on first recheck", duration)
		},
	}

	waitForCodexRateLimitReset(d, agentrunner.Session{Label: "iter-01", ID: "pane-1"}, base.Add(-time.Hour))
}
