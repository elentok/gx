package herdrrunner

import (
	"testing"
	"time"
)

func TestDetectRateLimit_MatchesKnownMessageVariants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		text      string
		wantToken string
	}{
		{
			name:      "session limit hit with reset time and zone",
			text:      "You've hit your session limit · resets 10:10am (UTC)",
			wantToken: "10:10am (UTC)",
		},
		{
			name:      "usage limit reached, no reset time",
			text:      "Claude usage limit reached",
			wantToken: "",
		},
		{
			name:      "usage limit reached, reset later in the line",
			text:      "Claude usage limit reached. Your limit will reset at 3pm (America/New_York).",
			wantToken: "3pm (America/New_York)",
		},
		{
			name:      "hour limit reached",
			text:      "5-hour limit reached ∙ resets 3pm",
			wantToken: "3pm",
		},
		{
			name:      "reset time comes from the hit line, not an earlier clock time",
			text:      "✻ Cogitated for 2m 42s · done 6:49 PM\nYou've hit your session limit · resets 10:50pm",
			wantToken: "10:50pm",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			token, matched := DetectRateLimit(tc.text)
			if !matched {
				t.Fatalf("DetectRateLimit(%q) matched = false, want true", tc.text)
			}
			if token != tc.wantToken {
				t.Errorf("DetectRateLimit(%q) token = %q, want %q", tc.text, token, tc.wantToken)
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
		"session limit resets at 3pm",
		"Approaching usage limit · resets at 10pm",
		"You've used 90% of your session limit · resets 10:50pm",
		"✻ Cogitated for 2m 42s · done 6:49 PM",
	}

	for _, text := range cases {
		if _, matched := DetectRateLimit(text); matched {
			t.Errorf("DetectRateLimit(%q) matched = true, want false", text)
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
			limit, matched := DetectCodexRateLimit(tc.text, now)
			if !matched {
				t.Fatalf("DetectCodexRateLimit(%q) matched = false, want true", tc.text)
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
		if _, matched := DetectCodexRateLimit(text, time.Now()); matched {
			t.Errorf("DetectCodexRateLimit(%q) matched = true, want false", text)
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
			got, ok := SecondsUntilReset(tc.token, now)
			if !ok {
				t.Fatalf("SecondsUntilReset(%q) ok = false, want true", tc.token)
			}
			if got != tc.want {
				t.Errorf("SecondsUntilReset(%q) = %v, want %v", tc.token, got, tc.want)
			}
		})
	}
}

func TestSecondsUntilReset_HonorsZoneSuffixElseNowsZone(t *testing.T) {
	t.Parallel()
	idt := time.FixedZone("IDT", 3*60*60)
	now := time.Date(2026, 10, 6, 18, 49, 0, 0, idt) // 15:49 UTC

	cases := []struct {
		name  string
		token string
		want  time.Duration
	}{
		{"bare time is in now's zone", "10:50pm", 4*time.Hour + time.Minute},
		{"UTC suffix", "6pm (UTC)", 2*time.Hour + 11*time.Minute},
		{"IANA zone suffix", "11pm (Asia/Jerusalem)", 4*time.Hour + 11*time.Minute},
		{"unknown zone falls back to now's zone", "10:50pm (Mars/Base)", 4*time.Hour + time.Minute},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := SecondsUntilReset(tc.token, now)
			if !ok {
				t.Fatalf("SecondsUntilReset(%q) ok = false, want true", tc.token)
			}
			if got != tc.want {
				t.Errorf("SecondsUntilReset(%q) = %v, want %v", tc.token, got, tc.want)
			}
		})
	}
}

func TestSecondsUntilReset_UnparseableToken_ReturnsNotOK(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

	for _, token := range []string{"", "not a time", "midnight"} {
		if _, ok := SecondsUntilReset(token, now); ok {
			t.Errorf("SecondsUntilReset(%q) ok = true, want false", token)
		}
	}
}
