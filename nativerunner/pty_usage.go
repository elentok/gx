package nativerunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
)

// RateLimit reads the limit from the transcript fact the holder reports, not
// the screen. A message without a parseable reset time gives a zero time.
func (p *Pty) RateLimit(s agentrunner.Session) (time.Time, bool, error) {
	ss, err := p.live(s)
	if err != nil {
		return time.Time{}, false, err
	}
	message, limited, err := ss.holder.RateLimitMessage()
	if err != nil {
		return time.Time{}, false, p.died(ss, err)
	}
	if !limited {
		return time.Time{}, false, nil
	}
	now := time.Now()
	if d, ok := claudeui.SecondsUntilReset(claudeui.ResetToken(message), now); ok {
		return now.Add(d), true, nil
	}
	return p.usageRecordReset(ss.session.Label), true, nil
}

// usageLimitPercent is the fixed usage at which a record window counts as the
// limit that fired.
const usageLimitPercent = 98

// usageRecordReset is the latest resets_at among label's record windows at
// usageLimitPercent or more, or zero when there is no such window. A missing
// or unreadable record gives zero: the record only refines the message.
func (p *Pty) usageRecordReset(label string) time.Time {
	data, err := os.ReadFile(filepath.Join(p.Root, label, UsageFile))
	if err != nil {
		return time.Time{}
	}
	var rec UsageRecord
	if json.Unmarshal(data, &rec) != nil {
		return time.Time{}
	}
	var latest int64
	for _, w := range []*UsageWindow{rec.FiveHour, rec.SevenDay} {
		if w != nil && w.UsedPercentage >= usageLimitPercent {
			latest = max(latest, w.ResetsAt)
		}
	}
	if latest == 0 {
		return time.Time{}
	}
	return time.Unix(latest, 0)
}
