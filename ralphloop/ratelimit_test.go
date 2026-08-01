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

func TestWaitForRateLimitReset_ParseableToken_SleepsThroughReset(t *testing.T) {
	var slept []time.Duration
	d := Deps{
		Sleep: func(d time.Duration) { slept = append(slept, d) },
	}

	waitForRateLimitReset(d, "pane-1", "3pm")

	if len(slept) != 1 {
		t.Fatalf("Sleep called %d times, want exactly 1 (no fallback polling needed)", len(slept))
	}
}

func TestWaitForRateLimitReset_UnparseableToken_PollsUntilMessageClears(t *testing.T) {
	var slept int
	calls := 0
	d := Deps{
		Sleep: func(time.Duration) { slept++ },
		ReadPaneRecent: func(pane string) (string, error) {
			calls++
			if calls < 3 {
				return "Claude usage limit reached", nil
			}
			return "working on the task now", nil
		},
	}

	waitForRateLimitReset(d, "pane-1", "")

	if calls != 3 {
		t.Errorf("ReadPaneRecent called %d times, want 3 (poll until message clears)", calls)
	}
	if slept != 3 {
		t.Errorf("Sleep called %d times, want 3", slept)
	}
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
