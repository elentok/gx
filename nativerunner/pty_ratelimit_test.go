package nativerunner_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

// waitRateLimit polls p.RateLimit until limited matches or it times out.
func waitRateLimit(t *testing.T, p *nativerunner.Pty, s agentrunner.Session, limited bool) time.Time {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		at, got, err := p.RateLimit(s)
		if err != nil {
			t.Fatal(err)
		}
		if got == limited {
			return at
		}
		if time.Now().After(deadline) {
			t.Fatalf("RateLimit limited = %v; want %v", got, limited)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func startRateLimitSession(t *testing.T) (*nativerunner.Pty, agentrunner.Session) {
	t.Helper()
	p := newPty(t)
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Stop(s) })
	return p, s
}

func TestPty_RateLimitReadsTheTranscriptResetTime(t *testing.T) {
	t.Parallel()
	p, s := startRateLimitSession(t)
	if _, limited, err := p.RateLimit(s); err != nil || limited {
		t.Fatalf("RateLimit before an error = %v, %v; want not limited", limited, err)
	}

	ptyControl(t, p, s, agentfake.Control{Action: agentfake.ActionAPIError, Error: "rate_limit", Message: "You've hit your limit · resets 3pm"})
	at := waitRateLimit(t, p, s, true)

	d, ok := claudeui.SecondsUntilReset("3pm", time.Now())
	if !ok {
		t.Fatal("SecondsUntilReset(3pm) not ok")
	}
	if want := time.Now().Add(d); at.Sub(want).Abs() > time.Minute {
		t.Errorf("reset = %v; want next 3pm %v", at, want)
	}
}

func TestPty_RateLimitWithoutResetIsLimitedWithZeroTime(t *testing.T) {
	t.Parallel()
	p, s := startRateLimitSession(t)
	ptyControl(t, p, s, agentfake.Control{Action: agentfake.ActionAPIError, Error: "rate_limit", Message: "You've hit your limit"})
	if at := waitRateLimit(t, p, s, true); !at.IsZero() {
		t.Errorf("reset = %v; want zero", at)
	}
}

func TestPty_RateLimitIgnoresOtherAPIErrors(t *testing.T) {
	t.Parallel()
	p, s := startRateLimitSession(t)
	ptyControl(t, p, s, agentfake.Control{Action: agentfake.ActionAPIError, Error: "authentication_error", Status: 401, Message: "Invalid API key"})
	time.Sleep(time.Second)
	if _, limited, err := p.RateLimit(s); err != nil || limited {
		t.Errorf("RateLimit = %v, %v; want not limited", limited, err)
	}
}

func TestPty_RateLimitClearsWhenANewTurnStarts(t *testing.T) {
	t.Parallel()
	p, s := startRateLimitSession(t)
	ptyControl(t, p, s, agentfake.Control{Action: agentfake.ActionAPIError, Error: "rate_limit", Message: "You've hit your limit · resets 3pm"})
	waitRateLimit(t, p, s, true)

	if err := p.Prompt(s, "go on"); err != nil {
		t.Fatal(err)
	}
	waitRateLimit(t, p, s, false)
}
