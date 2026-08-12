package ralphloop

import (
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
)

func TestDetectRateLimit_MatchesKnownMessageVariants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		text      string
		wantToken string
	}{
		{
			name:      "session limit hit with reset time",
			text:      "You've hit your session limit · resets 10:10am (UTC)",
			wantToken: "10:10am",
		},
		{
			name:      "usage limit reached, no reset time",
			text:      "Claude usage limit reached",
			wantToken: "",
		},
		{
			name:      "reset-first phrasing",
			text:      "session limit resets at 3pm",
			wantToken: "3pm",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			token, matched := detectRateLimit(tc.text)
			if !matched {
				t.Fatalf("detectRateLimit(%q) matched = false, want true", tc.text)
			}
			if token != tc.wantToken {
				t.Errorf("detectRateLimit(%q) token = %q, want %q", tc.text, token, tc.wantToken)
			}
		})
	}
}

func TestDetectRateLimit_DoesNotMatchIncidentalMentions(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"Added a rate limit of 100 requests per minute",
		"blocked: waiting for your permission to run this command",
		"agent status: idle",
	}

	for _, text := range cases {
		if _, matched := detectRateLimit(text); matched {
			t.Errorf("detectRateLimit(%q) matched = true, want false", text)
		}
	}
}

func TestDetectCodexRateLimit_ClassifiesKnownMessages(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		text      string
		wantReset time.Time
	}{
		{
			name:      "absolute reset timestamp",
			text:      "  ■ You've hit your usage limit. Upgrade to Pro or try again at Aug 5, 2026 3:06 PM.",
			wantReset: time.Date(2026, 8, 5, 15, 6, 0, 0, time.UTC),
		},
		{
			name:      "relative reset duration",
			text:      "You've hit your usage limit. Try again in 2 hours 33 minutes 12 seconds.",
			wantReset: now.Add(2*time.Hour + 33*time.Minute + 12*time.Second),
		},
		{
			name: "reset omitted",
			text: "You've hit your usage limit.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			limit, matched := detectCodexRateLimit(tc.text, now)
			if !matched {
				t.Fatalf("detectCodexRateLimit(%q) matched = false, want true", tc.text)
			}
			if limit.Quota != "usage" {
				t.Errorf("quota = %q, want usage", limit.Quota)
			}
			if !limit.ResetAt.Equal(tc.wantReset) {
				t.Errorf("reset = %v, want %v", limit.ResetAt, tc.wantReset)
			}
		})
	}
}

func TestDetectCodexRateLimit_RejectsBlockedAndIncidentalText(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"",
		"blocked: waiting for your permission to run this command",
		"rate limit reached while calling a test server",
		"We should detect You've hit your usage limit. in pane output",
		"Claude usage limit reached",
	} {
		if _, matched := detectCodexRateLimit(text, time.Now()); matched {
			t.Errorf("detectCodexRateLimit(%q) matched = true, want false", text)
		}
	}
}

func TestSecondsUntilReset_ParsesClockTimeRollingToNextDayIfPassed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		token string
		want  time.Duration
	}{
		{"later today, with minutes", "10:10am", 70 * time.Minute},
		{"already passed today, rolls to tomorrow", "3am", 18 * time.Hour},
		{"exactly now rolls to tomorrow", "9am", 24 * time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := secondsUntilReset(tc.token, now)
			if !ok {
				t.Fatalf("secondsUntilReset(%q) ok = false, want true", tc.token)
			}
			if got != tc.want {
				t.Errorf("secondsUntilReset(%q) = %v, want %v", tc.token, got, tc.want)
			}
		})
	}
}

func TestSecondsUntilReset_UnparseableToken_ReturnsNotOK(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

	for _, token := range []string{"", "not a time", "midnight"} {
		if _, ok := secondsUntilReset(token, now); ok {
			t.Errorf("secondsUntilReset(%q) ok = true, want false", token)
		}
	}
}

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
