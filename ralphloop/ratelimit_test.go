package ralphloop

import (
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
)

func TestDetectRateLimit_MatchesKnownMessageVariants(t *testing.T) {
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

func TestSecondsUntilReset_ParsesClockTimeRollingToNextDayIfPassed(t *testing.T) {
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
	now := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

	for _, token := range []string{"", "not a time", "midnight"} {
		if _, ok := secondsUntilReset(token, now); ok {
			t.Errorf("secondsUntilReset(%q) ok = true, want false", token)
		}
	}
}

func TestWaitForClaudeRateLimitReset_ParseableToken_ReturnsOnceDeadlinePasses(t *testing.T) {
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
		Now:            func() time.Time { return current },
		ResumeSignaled: func(string) (bool, error) { return false, nil },
	}

	waitForClaudeRateLimitReset(d, g, "t1", "/resume-signal", "pane-1", "9:05am")

	if slept == 0 {
		t.Errorf("Sleep never called, want at least one poll before the deadline check")
	}
}

func TestWaitForClaudeRateLimitReset_UnparseableToken_PollsUntilMessageClears(t *testing.T) {
	g := NewGate()
	g.pause("t1", "rate limit detected")

	current := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	calls := 0
	d := Deps{
		// Advance wall-clock time on each Sleep so the coarse
		// rateLimitPollInterval text-recheck cadence is actually reached
		// without a real sleep.
		Sleep:          func(time.Duration) { current = current.Add(rateLimitPollInterval) },
		Now:            func() time.Time { return current },
		ResumeSignaled: func(string) (bool, error) { return false, nil },
		ReadPaneRecent: func(pane string) (string, error) {
			calls++
			if calls < 3 {
				return "Claude usage limit reached", nil
			}
			return "working on the task now", nil
		},
	}

	waitForClaudeRateLimitReset(d, g, "t1", "/resume-signal", "pane-1", "")

	if calls != 3 {
		t.Errorf("ReadPaneRecent called %d times, want 3 (poll until message clears)", calls)
	}
}

func TestWaitForClaudeRateLimitReset_ResumeSignal_ReturnsImmediately(t *testing.T) {
	g := NewGate()
	g.pause("t1", "rate limit detected")

	d := Deps{
		Sleep:          func(time.Duration) { t.Fatal("should not sleep: resume signal already present") },
		ResumeSignaled: func(string) (bool, error) { return true, nil },
		Now:            time.Now,
	}

	waitForClaudeRateLimitReset(d, g, "t1", "/resume-signal", "pane-1", "3pm")
}

func TestWaitForClaudeRateLimitReset_ForceResumed_ReturnsImmediately(t *testing.T) {
	g := NewGate()
	g.pause("t1", "rate limit detected")
	g.ForceResume("t1")

	d := Deps{
		Sleep:          func(time.Duration) { t.Fatal("should not sleep: already force-resumed") },
		ResumeSignaled: func(string) (bool, error) { return false, nil },
		Now:            time.Now,
	}

	waitForClaudeRateLimitReset(d, g, "t1", "/resume-signal", "pane-1", "3pm")
}

func TestWaitForCodexRateLimitReset_MissingResetPollsUntilQuotaClears(t *testing.T) {
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

func TestWaitForCodexRateLimitReset_SleepsPastResetThenReobserves(t *testing.T) {
	d := Deps{}
	var sleeps []time.Duration
	checks := 0
	d.Sleep = func(duration time.Duration) { sleeps = append(sleeps, duration) }
	d.ReadCodexRateLimit = func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
		checks++
		return codexsession.RateLimit{}, false, nil
	}

	waitForCodexRateLimitReset(d, "/repo/iter-01", "session-1", codexsession.RateLimit{
		Quota: "secondary", ResetAt: time.Now().Add(2 * time.Second),
	})

	if len(sleeps) != 1 || sleeps[0] < rateLimitResetBuffer {
		t.Errorf("sleeps = %v, want a sleep through the reset plus buffer", sleeps)
	}
	if checks != 1 {
		t.Errorf("quota rechecks = %d, want 1 after reset", checks)
	}
}
