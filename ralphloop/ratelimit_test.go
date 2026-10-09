package ralphloop

import (
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
)

func TestWaitForClaudeRateLimitReset_ParseableToken_ReturnsOnceDeadlinePasses(t *testing.T) {
	t.Parallel()
	g := NewGate()
	g.pause("t1", "rate limit detected, resets 3pm")

	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	current := base
	slept := 0
	d := Deps{
		Sleep: func(time.Duration) {
			slept++
			current = current.Add(rateLimitPollInterval)
		},
		Now: func() time.Time { return current },
	}

	waitForClaudeRateLimitReset(d, g, "t1", "pane-1", "9:05am")

	if slept == 0 {
		t.Errorf("Sleep never called, want at least one poll before the deadline check")
	}
}

func TestWaitForClaudeRateLimitReset_UnparseableToken_PollsUntilMessageClears(t *testing.T) {
	t.Parallel()
	g := NewGate()
	g.pause("t1", "rate limit detected")

	current := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	calls := 0
	d := Deps{
		// Advance wall-clock time on each Sleep so the coarse
		// rateLimitPollInterval text-recheck cadence is actually reached
		// without a real sleep.
		Sleep: func(time.Duration) { current = current.Add(rateLimitPollInterval) },
		Now:   func() time.Time { return current },
		ReadPaneRecent: func(pane string) (string, error) {
			calls++
			if calls < 3 {
				return "Claude usage limit reached", nil
			}
			return "working on the task now", nil
		},
	}

	waitForClaudeRateLimitReset(d, g, "t1", "pane-1", "")

	if calls != 3 {
		t.Errorf("ReadPaneRecent called %d times, want 3 (poll until message clears)", calls)
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

	waitForClaudeRateLimitReset(d, g, "t1", "pane-1", "3pm")
}

func TestWaitForCodexRateLimitReset_MissingResetPollsUntilQuotaClears(t *testing.T) {
	t.Parallel()
	d := Deps{}
	var sleeps []time.Duration
	checks := 0
	d.Sleep = func(duration time.Duration) { sleeps = append(sleeps, duration) }
	d.ReadCodexRateLimit = func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
		checks++
		return codexsession.RateLimit{}, checks == 1, nil
	}

	waitForCodexRateLimitReset(d, "/repo/iter-01", "session-1", codexsession.RateLimit{Quota: "primary"})

	if len(sleeps) != 2 || sleeps[0] != rateLimitPollInterval || sleeps[1] != rateLimitPollInterval {
		t.Errorf("sleeps = %v, want two %v polls", sleeps, rateLimitPollInterval)
	}
}

func TestWaitForCodexRateLimitReset_MissingResetAt_NeverClears_BoundedThenReturns(t *testing.T) {
	t.Parallel()
	d := Deps{}
	checks := 0
	d.Sleep = func(time.Duration) {}
	d.ReadCodexRateLimit = func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
		checks++
		return codexsession.RateLimit{}, true, nil
	}

	done := make(chan struct{})
	go func() {
		waitForCodexRateLimitReset(d, "/repo/iter-01", "session-1", codexsession.RateLimit{Quota: "primary"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForCodexRateLimitReset never returned: an unchanging exhausted snapshot polled indefinitely")
	}

	if checks != codexRateLimitMaxRepolls {
		t.Errorf("quota rechecks = %d, want %d (bounded)", checks, codexRateLimitMaxRepolls)
	}
}

func TestWaitForCodexRateLimitReset_SleepsPastResetThenReobserves(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	d := Deps{Now: func() time.Time { return base }}
	var sleeps []time.Duration
	checks := 0
	d.Sleep = func(duration time.Duration) { sleeps = append(sleeps, duration) }
	d.ReadCodexRateLimit = func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
		checks++
		return codexsession.RateLimit{}, false, nil
	}

	waitForCodexRateLimitReset(d, "/repo/iter-01", "session-1", codexsession.RateLimit{
		Quota: "secondary", ResetAt: base.Add(2 * time.Second),
	})

	if len(sleeps) != 1 || sleeps[0] < rateLimitResetBuffer {
		t.Errorf("sleeps = %v, want a sleep through the reset plus buffer", sleeps)
	}
	if checks != 1 {
		t.Errorf("quota rechecks = %d, want 1 after reset", checks)
	}
}

func TestWaitForCodexRateLimitReset_StaleResetAt_NoWaitAndBoundedRepolls(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	d := Deps{Now: func() time.Time { return base }}
	var sleeps []time.Duration
	checks := 0
	d.Sleep = func(duration time.Duration) { sleeps = append(sleeps, duration) }
	d.ReadCodexRateLimit = func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
		checks++
		return codexsession.RateLimit{}, true, nil
	}

	// ResetAt is already in the past relative to the deterministic clock —
	// an immutable pre-reset record from a deadline that already passed.
	waitForCodexRateLimitReset(d, "/repo/iter-01", "session-1", codexsession.RateLimit{
		Quota: "secondary", ResetAt: base.Add(-time.Hour),
	})

	if len(sleeps) != codexRateLimitMaxRepolls {
		t.Errorf("sleeps = %v, want %d bounded repoll sleeps and no deadline wait", sleeps, codexRateLimitMaxRepolls)
	}
	// One check right after the (already-past) deadline, plus the bounded
	// repoll loop.
	if checks != codexRateLimitMaxRepolls+1 {
		t.Errorf("quota rechecks = %d, want %d", checks, codexRateLimitMaxRepolls+1)
	}
}

func TestWaitForCodexRateLimitReset_ClearedRightAfterDeadline_ReturnsWithoutRepolling(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	d := Deps{Now: func() time.Time { return base }}
	var sleeps []time.Duration
	checks := 0
	d.Sleep = func(duration time.Duration) { sleeps = append(sleeps, duration) }
	d.ReadCodexRateLimit = func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
		checks++
		return codexsession.RateLimit{}, false, nil
	}

	waitForCodexRateLimitReset(d, "/repo/iter-01", "session-1", codexsession.RateLimit{
		Quota: "secondary", ResetAt: base.Add(-time.Hour),
	})

	if len(sleeps) != 0 {
		t.Errorf("sleeps = %v, want none: reset already passed and quota cleared on first recheck", sleeps)
	}
	if checks != 1 {
		t.Errorf("quota rechecks = %d, want 1", checks)
	}
}
