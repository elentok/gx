package herdrrunner

import (
	"testing"
	"time"
)

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
