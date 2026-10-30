package nativerunner_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// waitRateLimit polls p.RateLimit until limited matches or it times out.
func waitRateLimit(t *testing.T, p *nativerunner.Pty, s agentrunner.Session, limited bool) time.Time {
	t.Helper()
	var at time.Time
	if !testutil.Poll(10*time.Second, 50*time.Millisecond, func() bool {
		var got bool
		var err error
		at, got, err = p.RateLimit(s)
		if err != nil {
			t.Fatal(err)
		}
		return got == limited
	}) {
		t.Fatalf("RateLimit limited = %v; want %v", !limited, limited)
	}
	return at
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

// writeUsageRecord writes the usage record the statusline would, by hand.
func writeUsageRecord(t *testing.T, p *nativerunner.Pty, s agentrunner.Session, rec nativerunner.UsageRecord) {
	t.Helper()
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, s.Label, nativerunner.UsageFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func limitWithoutReset(t *testing.T, p *nativerunner.Pty, s agentrunner.Session) {
	t.Helper()
	ptyControl(t, p, s, agentfake.Control{Action: agentfake.ActionAPIError, Error: "rate_limit", Message: "You've hit your limit"})
}

func TestPty_RateLimitFallsBackToTheLatestResetAtOrAbove98(t *testing.T) {
	t.Parallel()
	p, s := startRateLimitSession(t)
	soon := time.Now().Add(time.Hour).Unix()
	later := time.Now().Add(48 * time.Hour).Unix()
	writeUsageRecord(t, p, s, nativerunner.UsageRecord{
		FiveHour: &nativerunner.UsageWindow{UsedPercentage: 99, ResetsAt: soon},
		SevenDay: &nativerunner.UsageWindow{UsedPercentage: 100, ResetsAt: later},
	})
	limitWithoutReset(t, p, s)
	if at := waitRateLimit(t, p, s, true); at.Unix() != later {
		t.Errorf("reset = %v; want %v", at, time.Unix(later, 0))
	}
}

func TestPty_RateLimitMessageResetWinsOverTheRecord(t *testing.T) {
	t.Parallel()
	p, s := startRateLimitSession(t)
	writeUsageRecord(t, p, s, nativerunner.UsageRecord{
		FiveHour: &nativerunner.UsageWindow{UsedPercentage: 100, ResetsAt: time.Now().Add(100 * time.Hour).Unix()},
	})
	ptyControl(t, p, s, agentfake.Control{Action: agentfake.ActionAPIError, Error: "rate_limit", Message: "You've hit your limit · resets 3pm"})
	at := waitRateLimit(t, p, s, true)
	if at.After(time.Now().Add(25 * time.Hour)) {
		t.Errorf("reset = %v; want the message's 3pm, not the record", at)
	}
}

func TestPty_RateLimitIgnoresRecordWindowsUnder98(t *testing.T) {
	t.Parallel()
	p, s := startRateLimitSession(t)
	writeUsageRecord(t, p, s, nativerunner.UsageRecord{
		FiveHour: &nativerunner.UsageWindow{UsedPercentage: 97, ResetsAt: time.Now().Add(time.Hour).Unix()},
	})
	limitWithoutReset(t, p, s)
	if at := waitRateLimit(t, p, s, true); !at.IsZero() {
		t.Errorf("reset = %v; want zero", at)
	}
}

func TestPty_RateLimitReturnsAPastRecordResetUnchanged(t *testing.T) {
	t.Parallel()
	p, s := startRateLimitSession(t)
	past := time.Now().Add(-2 * time.Hour).Unix()
	writeUsageRecord(t, p, s, nativerunner.UsageRecord{
		SeenAt:   time.Now().Add(-24 * time.Hour),
		FiveHour: &nativerunner.UsageWindow{UsedPercentage: 98, ResetsAt: past},
	})
	limitWithoutReset(t, p, s)
	if at := waitRateLimit(t, p, s, true); at.Unix() != past {
		t.Errorf("reset = %v; want %v", at, time.Unix(past, 0))
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
